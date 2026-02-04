---
Date: 2026-02-01
Title: Audio Daemon ffplay Backend
Status: completed
Dependencies: None
---

## Description
Bounce should no longer convert and copy files. We can use audio daemon with ffplay to use original file.

The original beep library had issues with seeking (static/glitches). FFplay handles 32-bit float WAV files natively and provides clean seeking.

## Implementation Summary
The audio daemon was rewritten to use ffplay instead of the gopxl/beep library:

1. **Audio Playback**: Uses ffplay subprocess for playback
2. **Duration Detection**: Uses ffprobe to get audio file duration
3. **Seeking**: Works by stopping and restarting ffplay at the new position
4. **Original Files**: Can now play 32-bit float WAV files directly (no conversion needed)

## Files Changed
- `audio-daemon/server.go` - Rewrote to use ffplay/ffprobe instead of beep
- `plan/Shortcuts.md` - Added audio daemon CLI commands and debugging tips

## Verification
- Audio plays cleanly with ffplay backend
- Seeking works without static/glitches
- Original 32-bit float files play directly
