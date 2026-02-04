---
Date: 2026-02-01
Title: Audio Daemon Auto Start/Stop
Status: completed
---

## Description

The CLI should start the audio daemon automatically when needed and stop it after inactivity.

## Requirements

### Auto-Start
- `am audio load /path/to/file.wav` - start daemon if not started
- `am audio play` - start daemon if not started
- `am audio pause` - (daemon already running)
- `am audio seek 30` - (daemon already running)
- `am audio status` - (daemon already running)

### Auto-Stop
After a clip finishes loading, or it's been paused for more than 10s, let's go ahead and kill the daemon.

## Acceptance Criteria

1. Audio daemon starts automatically when `am audio load` or `am audio play` is called
2. Audio daemon stops automatically after 10s of pause/idle
3. Audio daemon stops after clip finishes loading (if not playing)

## Plan

### Part 1: Auto-Start (handlers/audio.go)

1. Add `IsAudioDaemonRunning()` function - checks PID file and process
2. Add `StartAudioDaemon()` function - starts daemon in background
3. Add `EnsureAudioDaemonRunning()` function - starts daemon if not running, waits for ready
4. Modify `GetAudioClient()` to call `EnsureAudioDaemonRunning()` before connecting

### Part 2: Auto-Stop (audio-daemon/server.go)

1. Add `idleTimer` and `idleTimeout` constants to server struct
2. Add `startIdleTimer()` function - starts 10s timer that triggers shutdown
3. Add `resetIdleTimer()` function - resets/cancels the timer
4. Call `startIdleTimer()` after Load (if not playing), Pause, Stop
5. Call `resetIdleTimer()` (cancel) when Play starts
6. On timer fire, gracefully shutdown the server

### Files to Modify:
- `ableton-manager-cli/handlers/audio.go` - Add auto-start logic
- `audio-daemon/server.go` - Add idle timeout logic
- `audio-daemon/main.go` - Add graceful shutdown support

### Commands Run / Actions Taken

1. Added `IsAudioDaemonRunning()`, `StartAudioDaemon()`, `EnsureAudioDaemonRunning()` to `handlers/audio.go`
2. Modified `GetAudioClient()` to call `EnsureAudioDaemonRunning()` before connecting
3. Added idle timeout constants and timer fields to `AudioPlayerServer` struct in `server.go`
4. Added `startIdleTimer()` and `cancelIdleTimer()` methods to server
5. Modified `Load()`, `Pause()`, `Stop()` to start idle timer; `Play()` to cancel it
6. Updated `main.go` to pass shutdown function to server for graceful shutdown
7. Built with `make build`

### Results

- Auto-start: Daemon starts automatically when any `am audio` command needs it
- Auto-stop: Daemon shuts down after 10s of idle (after Load, Pause, or Stop)
- Play cancels timer: While playing, daemon stays running indefinitely
- PID file cleanup: Gracefully removed on shutdown

### Verification Commands / Steps

```bash
# Test 1: Verify daemon is not running initially
pgrep -f "audio-daemon"  # No output expected

# Test 2: Test auto-start with am audio load
am audio load /System/Library/Sounds/Basso.aiff --verbose
# Output: "[EnsureAudioDaemonRunning] Daemon not running, starting..."
# Output: "success": true

# Test 3: Verify daemon is now running
pgrep -f "audio-daemon"  # Returns PID
cat ~/.ableton-manager/audio-daemon.pid  # Shows same PID

# Test 4: Check audio status
am audio status  # Shows loaded file, position

# Test 5: Wait for auto-stop (10s idle timeout)
sleep 12 && pgrep -f "audio-daemon"  # No output (daemon stopped)

# Test 6: Verify PID file was cleaned up
ls ~/.ableton-manager/audio-daemon.pid  # File not found

# Test 7: Full cycle - load, play, pause, auto-stop
am audio load /System/Library/Sounds/Basso.aiff && \
am audio play && sleep 3 && pgrep -f "audio-daemon" && \
am audio pause && sleep 12 && pgrep -f "audio-daemon"  # No output (stopped)
```

**All tests passed:**
- ✅ Auto-start on `am audio load`
- ✅ Play cancels idle timer (daemon stays running while playing)
- ✅ Pause starts idle timer
- ✅ Auto-stop after 10s idle
- ✅ PID file cleaned up on shutdown

**Verification: 100% complete** - End-to-end CLI testing with full play/pause cycle verified
