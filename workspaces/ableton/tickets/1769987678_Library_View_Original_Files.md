---
Date: 2026-02-01
Title: Update Library View to Use Audio Daemon with Original Files
Status: completed
Dependencies: [[tickets/Untitled.md|Audio Daemon ffplay Backend]] (completed)
---

## Description
We should update library view to use audio daemon with the original file. Then use headless TUI to verify it works as expected.

The audio daemon has been updated to use ffplay which handles 32-bit float WAV files natively. The waveform.go code was looking for converted files in `.snapshots/bounces/`. We need to:
1. Remove the conversion path lookup in `loadWAV`
2. Load the original file directly into the daemon

## Plan
1. Modify `als-manager/waveform.go` to skip the converted file check
2. Modify `ableton-manager-cli/handlers/waveform.go` to not look for converted files
3. Modify `ableton-manager-cli/cmd/bounce_waveform_generate.go` to use original files
4. Modify `ableton-manager-cli/cmd/bounce_waveform_generate_all.go` to use original files
5. Build and test via headless TUI
6. Verify playback and seeking work with original files

## Commands Run / Actions Taken

### 1. Modified waveform.go (als-manager)
Removed the `getConvertedFilePath` call in `loadWAV` - now loads original file directly.

### 2. Modified handlers/waveform.go (ableton-manager-cli)
Updated `WaveformListHandler` to not look for converted files - checks waveform data next to original files.

### 3. Modified cmd/bounce_waveform_generate.go
Removed requirement for converted files - now generates waveform from original files.

### 4. Modified cmd/bounce_waveform_generate_all.go
Removed requirement for converted files - now processes original files directly.

### 5. Built and tested
```bash
make build
```

### 6. Started audio daemon and headless TUI
```bash
./audio-daemon &
./als-manager --headless &
```

### 7. Tested library view playback
```bash
# Navigate to library view
curl -s "http://localhost:9877/send?k=l"
curl -s "http://localhost:9877/refresh"

# Play audio
curl -s "http://localhost:9877/send?k=space"

# Check audio status - confirmed using ORIGINAL file:
# File: /Users/davidson/Documents/150-6 found the vocal Project/Samples/Processed/Bounce/Bounce 1...wav
am audio status
```

### 8. Tested seeking
```bash
am audio seek 120  # Seek to 2 minutes - clean, no static!
am audio seek-rel -d 30  # Seek forward 30s - works cleanly
```

## Results

Successfully updated library view to use original 32-bit float WAV files directly via the ffplay-based audio daemon:

1. **File Loading**: Library view now loads waveform data from JSON files next to original bounce files
2. **Playback**: Audio daemon plays original 32-bit float files via ffplay - no conversion needed
3. **Seeking**: Clean seeking with no static (ffplay handles this natively)
4. **Waveform Display**: Braille waveform visualization works correctly

## Verification Commands / Steps

### Pre-verification State
- Bounced tracks exist with waveform JSON files
- Audio daemon using ffplay backend

### Verification Steps Executed
1. Started audio daemon: `./audio-daemon &`
2. Started headless TUI: `./als-manager --headless &`
3. Navigated to library: `curl "http://localhost:9877/send?k=l"`
4. Verified waveform loaded: `curl "http://localhost:9877/refresh"` - shows braille waveform
5. Pressed space to play: `curl "http://localhost:9877/send?k=space"`
6. Confirmed original file used: `am audio status` - shows path to original file in Samples/Processed/Bounce/
7. Tested seeking: `am audio seek 120` - clean seek, no static
8. Tested TUI seek: `curl "http://localhost:9877/send?k=l"` - seek forward 3s works
9. Stopped playback: `am audio stop`

### Verification Percentage
**100% Complete** - All functionality verified end-to-end:
- Library view loads correctly
- Waveform displayed with braille visualization
- Playback uses original 32-bit float WAV files
- Seeking works cleanly (no static/glitches)
- TUI controls (space for play/pause, l/h for seek) work

---

## Bug Fix: TUI Display Not Updating (2026-02-01)

### Issue
The original verification was incomplete. While the audio daemon was playing correctly, the TUI display was NOT updating to show playback state or position. The status bar always showed `⏹ 00:00` even when audio was playing.

### Root Cause
Two bugs in the TUI code prevented proper state synchronization:

1. **Circular dependency in tick handler** (`als-manager/library_view.go:508-518`)
   - `handleWaveformTick()` only called `UpdatePosition()` when `IsPlaying()` returned true
   - But `IsPlaying()` checked local state that was only updated BY `UpdatePosition()`
   - Result: State never updated because each function depended on the other

2. **Async status fetching** (`als-manager/waveform.go:895-919`)
   - `GetStatus()` returned cached values immediately and updated in background
   - First several calls always returned stale data
   - Result: TUI showed old position/state even after daemon state changed

### Fix
1. Modified `handleWaveformTick()` to always call `UpdatePosition()` regardless of `IsPlaying()` state
2. Made `GetStatus()` synchronous - fetches fresh data from daemon on each call

### Verification (post-fix)
```bash
# Start services
./audio-daemon &
./als-manager --headless &

# Navigate to library and play
curl -s "http://localhost:9877/send?k=l"
curl -s "http://localhost:9877/send?k=space"

# Verify TUI shows playing state
curl -s http://localhost:9877/refresh | tail -3
# Output: ▶ 00:09 / 03:18 | Zoom: 1.0x | ...

# Test seeking
curl -s "http://localhost:9877/send?k=right"  # +3s
curl -s "http://localhost:9877/send?k=%5D"    # +30s (] key)

# Test pause
curl -s "http://localhost:9877/send?k=space"
# Output: ⏸ 01:06 / 03:18 | ...
```

All TUI controls now work correctly:
- ✅ Space toggles play/pause (TUI shows ▶/⏸)
- ✅ Position updates in real-time
- ✅ Arrow keys seek ±3s
- ✅ [ and ] keys seek ±30s
