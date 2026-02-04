# Shortcuts - Iteration Learnings

## Debugging OSC/gRPC Issues

When `am track inspect` shows `file_path: null` but raw OSC returns valid data:
1. Check for gRPC timeouts - tracks with many clips (100+) can timeout during batch fetching
2. Use `ao raw /live/track/get/<property>_batch <track_index> 0 10` to test raw OSC directly
3. The gRPC layer uses 2s timeout per batch - long file paths with special characters can cause timeouts

## Reliable vs Unreliable OSC Calls

**Reliable (fast, rarely timeout):**
- `GetNumArrangementClips(index)` - just returns a count
- `GetArrangementClipNames(index)` - string names are small
- `GetTrackIsGroup(index)`, `GetTrackHasMidiInput(index)` - simple booleans

**Unreliable (can timeout for tracks with many clips):**
- `GetArrangementClipFilePaths(index)` - file paths can be long with special characters

## Testing Bounce Operations

### Quick test for already-bounced track:
```bash
am bounce native <uuid> --verbose
```
Should immediately return `status: "already_bounced"` if track has "(Bounce)" in clip names.

### Find tracks with bounced clips:
```bash
am tracks --detailed | jq -r '.tracks[] | select(.arrangement_clips) | select([.arrangement_clips[].name] | any(contains("(Bounce)"))) | "\(.uuid[:8]): \(.name)"'
```

### Find audio tracks without bounced clips:
```bash
am tracks --detailed | jq '.tracks[] | select(.is_group == null) | select(.has_midi_input == null) | select(.arrangement_clips) | select([.arrangement_clips[].name] | all(contains("(Bounce)") | not)) | {name: .name, uuid: .uuid[:8]}'
```

## Build and Test Cycle

```bash
make build && am <command> --verbose
make test
```

## Dismissing Ableton Dialogs via AppleScript

### Inspecting dialog structure:
```bash
osascript -e 'tell application "System Events" to tell process "Live" to return entire contents of window 1'
```

### Dismiss "Save changes" dialog (Don't Save is button 1):
```bash
osascript -e 'tell application "System Events" to tell process "Live" to click button 1 of group 1 of window 1'
```

### Async AppleScript pattern (for commands that may block on dialogs):
```go
// Start command but don't wait - it may block on dialog
quitCmd := exec.Command("osascript", "-e", `tell application "Live" to quit`)
quitCmd.Start() // Don't use Run() or CombinedOutput()

// Then poll for dialog and dismiss while waiting for process to exit
```

## Reset Project for Fresh State

Use `am reset-project` to close and reopen the current project. Useful for:
- Testing with fresh project state
- Resetting after making changes you don't want to keep

```bash
# Reset and wait for reload (default)
am reset-project

# Reset without waiting
am reset-project --wait=false

# With verbose output
am reset-project --verbose
```

## Track Type Detection

In jq queries:
- `is_group == true` = group track
- `is_group == null` = non-group track
- `has_midi_input == true` = MIDI track
- `has_midi_input == null` = audio track (if also not group)

## Batch Bounce Operations

### Running batch bounce from CLI:
```bash
# Get all bounceable track UUIDs and bounce them
am bounce candidates | jq -r '.candidates[].uuid' | xargs am bounce native --async --verbose
```

**Note:** xargs may not find `am` alias. Use explicit UUIDs instead:
```bash
am bounce native uuid1 uuid2 uuid3 --async --verbose
```

### Check batch status:
```bash
# After async bounce returns batch_id
am bounce ls --batch <batch_id> --poll
```

### OSC connection degradation during batch:
The `ensureOSCConnection()` fix automatically restarts OSC daemon when connections degrade. Look for:
```
OSC connection degraded (...), restarting daemon...
OSC connection recovered after daemon restart
```

If recovery fails, manually restart daemon:
```bash
ao osc restart
```

## React Selection vs CLI Candidates

**React "smart select" (press 'a'):** Selects topmost bounceable parents, including groups with bounceable children
- May select 18 tracks including groups

**CLI `am bounce candidates`:** Only lists leaf tracks with actual clips to bounce
- May show 10 tracks (no groups)

To get accurate bounceable count, use CLI candidates:
```bash
am bounce candidates | jq '.count'
```

## Verifying Bounce Success

Check if a track was bounced by inspecting clip names:
```bash
am track inspect <uuid> | jq '.track.arrangement_clips[].name'
# Should show "(Bounce)" and/or "(Bounce tail)" if bounced
```

MIDI tracks change type to audio after bounce:
```bash
am tracks | jq '.tracks[] | select(.uuid == "<uuid>") | .has_midi_input'
# null = audio track (was bounced from MIDI)
```

## Headless TUI Mode

### Running Headless Bubble Tea
```bash
cd als-manager && ./als-manager --headless --headless-no-render 2>/tmp/headless.log &
```

### HTTP Endpoints:
- `GET /view` - Get cached view
- `GET /refresh` - Force capture and return view
- `GET /status` - JSON status with current view, tracks count, cursor position
- `GET /key?k=j` - Send keypress (j=down, k=up, b=bounce view, etc.)

### Testing Navigation:
```bash
curl -s "http://127.0.0.1:9877/key?k=b"  # Enter bounce view
sleep 3
curl -s "http://127.0.0.1:9877/refresh" | head -10  # View tracks
curl -s "http://127.0.0.1:9877/key?k=j"  # Move cursor down
curl -s "http://127.0.0.1:9877/refresh" | head -10  # Verify cursor moved
```


## Debugging Headless Mode

```bash
# View headless stderr logs
tail -f /tmp/headless.log

# Look for specific patterns
grep -E "SyncFromBounceState|BounceCursor|storeUIEvent" /tmp/headless.log

# Check if tracks loaded
curl -s http://127.0.0.1:9877/status | jq '.bounce_view.tracks_count'
```

**CPU Usage:** Use `--headless-no-render` to avoid ticker-based CPU spikes. Views are captured only on keypresses.

## Headless HTTP Handler Debugging

If HTTP endpoints connect but hang without responding:

1. **Symptom:** curl connects to localhost:9877 but times out, handler code never executes
2. **Root cause:** The default http.ServeMux may not properly route requests in certain goroutine scheduling scenarios
3. **Fix:** Wrap mux in a handler function:
```go
handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    mux.ServeHTTP(w, r)
})
http.ListenAndServe(addr, handler)  // Use handler, not mux
```

4. **Debug pattern:** Add stderr logging to verify handler execution:
```go
fmt.Fprintf(os.Stderr, "[HTTP] Request: %s %s\n", r.Method, r.URL.Path)
```

5. **program.Send() blocking:** If program.Send() blocks indefinitely, use goroutine with timeout:
```go
done := make(chan struct{})
go func() {
    program.Send(msg)
    close(done)
}()
select {
case <-done: // success
case <-time.After(2 * time.Second): // timeout
}
```

## Audio Daemon CLI Commands

```bash
# Start audio daemon if not running (auto-starts on load)
am audio load /path/to/file.wav

# Control playback
am audio play
am audio pause
am audio stop
am audio toggle  # Play/pause toggle

# Seeking
am audio seek 10        # Seek to 10 seconds
am audio seek-rel -d 5  # Seek forward 5 seconds
am audio seek-rel -d -5 # Seek backward 5 seconds (use -d flag for negative)

# Status
am audio status
```

**Note:** Use `-d` flag for seek-rel to handle negative numbers (flags starting with `-` conflict).

**Auto-start/stop behavior:**
- Daemon auto-starts on any `am audio` command (load, play, seek, status)
- Daemon auto-stops after 10s of idle (pause/stop or loaded without playing)
- Play cancels the idle timer, keeping daemon alive
- PID file cleaned up on graceful shutdown

## Fire-and-Forget Pattern for TUI Responsiveness

When TUI calls daemon operations, use goroutines to avoid blocking:

```go
func (c *DaemonClient) Play() error {
    go func() {
        client := NewAMClient()
        client.AudioPlay()
    }()
    return nil
}
```

This prevents lag in the TUI when daemon operations are slow.

## Audio File Formats

**Original bounce files** (in `Samples/Processed/Bounce/`) are 32-bit float WAV files. The audio daemon uses ffplay which handles these natively - no conversion needed.

**Waveform JSON** files should be generated next to the original bounce files:
```bash
am bounce waveform-generate 0  # Generate for track at index 0
am bounce waveform-generate-all  # Generate for all tracks
```

**Testing audio playback:**
```bash
# Load and play via daemon
am audio load "/path/to/bounce.wav"
am audio play
am audio seek 60  # Seek to 60 seconds
am audio status   # Check current state
```

## Headless Mode Message Routing

When adding new message types that need to be handled in `main_actions.go` (like `setProgramMsg`), be aware of view routing:

**Problem:** Messages get swallowed by view-specific routing before reaching main switch.

In `main_actions.go`, when `currentView == "bounceSelection"` or `currentView == "library"`, messages are first passed to the view's Update() method. Only messages explicitly listed continue to the main switch.

**If a message isn't being handled in headless mode:**
1. Check if it's in the allowed list for the active view routing (lines ~593-598 for bounceSelection, ~608-609 for library)
2. Add the message type to the switch case to allow it to continue to main switch

Example fix (line 594 in main_actions.go):
```go
case setProgramMsg, refreshCompleteMsg, scriptCompletedMsg, ...
// Added setProgramMsg to allow it to reach the main switch
```

**Also ensure headless mode sends required messages:**
- `RunHeadless()` in headless.go should mirror normal mode setup
- Example: Normal mode sends `setProgramMsg` via goroutine, headless mode should too

## Process Monitoring (CPU/Memory/Goroutines)

The library view now displays process stats in the footer:
```
CPU: 1.5% | Mem: 34.1MB | Goroutines: 48
```

Implementation in `als-manager/process_monitor.go`:
- Uses `syscall.Getrusage(syscall.RUSAGE_SELF, &rusage)` for CPU tracking
- Uses `runtime.ReadMemStats()` for memory
- Uses `runtime.NumGoroutine()` for goroutine count
- Samples every 500ms

**Diagnosing goroutine accumulation:**
```bash
# Start headless, do rapid operations, monitor goroutine count
./als-manager --headless -l &
curl -s http://localhost:9877/refresh | tail -2  # Initial count

# Rapid seeking test
for i in {1..30}; do curl -s "http://localhost:9877/send?k=right" > /dev/null; done
curl -s http://localhost:9877/refresh | tail -2  # Check accumulation

# Wait and verify cleanup
sleep 10
curl -s http://localhost:9877/refresh | tail -2  # Should reduce
```

**Known pattern:** Fire-and-forget goroutines in `waveform.go` (SeekRelative, Play, Pause, etc.) spawn goroutines that run `am audio` commands (~200ms each). Rapid seeking causes temporary accumulation that cleans up after commands complete.

## Non-Blocking Daemon Status Updates

**Problem:** Synchronous status fetch in tick handlers can block the main update loop, making navigation unresponsive during rapid operations.

**Solution:** `DaemonAudioClient.GetStatus()` uses async fetch with caching:
```go
// Returns cached values immediately (non-blocking)
// Starts background fetch only if not already in progress
if atomic.CompareAndSwapInt32(&c.fetchInProgress, 0, 1) {
    go func() {
        defer atomic.StoreInt32(&c.fetchInProgress, 0)
        // Fetch and update cache
    }()
}
```

**Key pattern:** Use atomic flags to prevent multiple concurrent fetches, return cached values immediately, update cache asynchronously.

**Testing navigation responsiveness:**
```bash
./als-manager --headless -l &
# Rapid scrubbing + navigation
for i in {1..15}; do curl -s "http://localhost:9877/send?k=right" > /dev/null; done
curl -s "http://localhost:9877/send?k=j" > /dev/null  # Should respond immediately
```

## Lipgloss Colors in Headless Mode

**By default, headless mode has no ANSI colors.** Use `--headless-color` flag to enable:

```bash
./als-manager --headless                     # No colors (default)
./als-manager --headless --headless-color    # With ANSI colors
```

**Verify colors in headless mode:**
```bash
curl -s 'http://localhost:9877/view/raw' | xxd | grep "38;2"
# Look for RGB color codes like:
# - [38;2;255;149;0m = Orange (#ff9500) for played waveform
# - [38;2;85;85;85m = Dark grey (#555555) for unplayed waveform
```

## Audio Daemon Idle Timeout Behavior

**Critical:** The audio daemon has a 10-second idle timeout. Audio loaded but not playing counts as "idle".

**Common issue:** Library view loads waveform + audio, user doesn't press play within 10s, daemon stops, play button doesn't work.

**Solution:** `TogglePlayPause()` now calls `EnsureLoaded()` before toggle to reload audio if daemon timed out.

**Testing daemon timeout:**
```bash
# Load audio, wait less than 10s - still loaded
am audio load "/path/to/audio.wav" && sleep 3 && am audio status

# Load audio, wait more than 10s - unloaded
am audio load "/path/to/audio.wav" && sleep 12 && am audio status
# Shows "No audio loaded"
```

**Key files for daemon reliability:**
- `waveform.go:EnsureLoaded()` - checks/reloads audio before operations
- `audio-daemon/server.go` - idle timeout constant (10s)
- `am_client.go:AudioLoad()` - auto-starts daemon if not running
