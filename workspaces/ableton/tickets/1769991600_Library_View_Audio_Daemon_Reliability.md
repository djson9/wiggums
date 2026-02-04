![[]]Date: 2026-02-01
Title: Library View Audio Daemon Reliability
Status: completed
Dependencies: None

## Description
The audio daemon does not consistently play audio in library view when using headless TUI mode. Users report having to manually load audio into the daemon because the headless TUI playback appears unreliable.

## Original Request
> It seems like there is some trouble getting the audio daemon consistently playing in library view. I have watched the LLM manually load audio into the audio daemon because the headless TUI did not appear to work reliably. Can you investigate and fix and verify the fix?

## Related Tickets
- [[tickets/1769990333_Audio_Daemon_Auto_Start_Stop.md|Audio Daemon Auto Start Stop]] (completed)
- [[tickets/1769990400_Library_View_Navigation_Blocked_During_Scrub.md|Library View Navigation Blocked During Scrub]] (completed)
- [[tickets/1769991000_Library_View_SoundCloud_Waveform.md|Library View SoundCloud Waveform]] (completed)

## Root Cause Analysis

### Problem
The audio daemon has a 10-second idle timeout. When the library view loads:
1. Waveform data loads and audio file is loaded into daemon
2. User hasn't pressed play yet (daemon is "idle")
3. After 10 seconds, daemon auto-stops due to idle timeout
4. User presses space to play
5. Toggle command restarts daemon, but audio file is NOT loaded
6. Nothing plays

### Evidence
```
[22:13:44.589] [DaemonAudioClient.Load] Goroutine started
[22:13:44.894] [DaemonAudioClient.Load] Loaded successfully

# After 12 seconds of idle:
$ am audio status
No audio loaded
```

## Fix Applied

Added `EnsureLoaded()` method to `DaemonAudioClient` (waveform.go) that:
1. Checks if audio is currently loaded in daemon
2. If not loaded (or wrong file), loads it before toggle
3. If already loaded, skips reload

Modified `TogglePlayPause()` to call `EnsureLoaded()` before toggling.

### Code Changes
- **waveform.go:342-365**: Modified `TogglePlayPause()` to call `EnsureLoaded(filePath)` before toggling
- **waveform.go:808-830**: Added `EnsureLoaded()` method that checks daemon status and reloads if needed

## Commands Run / Actions Taken
1. Added debug logging to trace audio load flow
2. Confirmed load was succeeding initially but daemon timing out
3. Verified 10-second idle timeout behavior:
   - Load audio, wait 3 seconds: audio still loaded
   - Load audio, wait 12 seconds: audio unloaded
4. Implemented `EnsureLoaded()` fix
5. Rebuilt and tested

## Results
- Audio playback now works reliably in library view even after daemon timeout
- Space bar correctly loads audio (if needed) and toggles play/pause
- Position display updates correctly during playback

## Verification Commands / Steps

### Test 1: Verify fix handles daemon timeout
```bash
# 1. Start headless TUI in library mode
./als-manager --headless -l &

# 2. Wait for initialization
sleep 5
curl -s 'http://localhost:9877/refresh' | tail -5  # Verify waveform shows

# 3. Wait beyond 10-second idle timeout
sleep 12
am audio status  # Should show "No audio loaded"

# 4. Press space to play
curl -s 'http://localhost:9877/send?k=space'
sleep 2

# 5. Verify audio is playing
am audio status  # Should show "State: ▶ Playing"
```

### Test 2: Verify pause works
```bash
# Press space again to pause
curl -s 'http://localhost:9877/send?k=space'
sleep 1
am audio status  # Should show "State: ⏸ Paused"
```

### Test 3: Verify logs show EnsureLoaded working
```bash
grep -i "EnsureLoaded" /Users/davidson/workspace/ableton-bouncer/development.log | tail -5
# First toggle: "Audio not loaded or different file, loading"
# Second toggle: "Audio already loaded"
```

### Verification completed: 100%
- [x] Audio plays after daemon timeout
- [x] Pause/resume works correctly
- [x] TUI shows correct playback state (▶ / ⏸)
- [x] Position display updates during playback
- [x] Logs confirm EnsureLoaded logic working
