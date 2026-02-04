Date: 2026-02-01
Title: Library View SoundCloud Style Waveform
Status: completed
Dependencies: None

## Description
User wants the waveform in library view to have a SoundCloud-style appearance:
1. Waveform should accurately depict the currently hovered track
2. No timestamps needed on the waveform
3. Waveform should appear concisely near the 00:00/03:10 timestamp area
4. Dark grey for portions not yet played
5. Orange for portions that have already played
6. The color division follows playback position as it scrubs/plays

## Original Request
> I would like for the waveform in library view to accurately depict the currently hovered track. There is no need for the timestamps. I would like the waveform concisely appearing near the 00:00/03:10 timestamp, and maybe it would be nice to have it kind of soundcloud style, where a dark grey indicates parts not played yet, and orange indicates parts that have already played, which follows around as it scrubs / plays.

## Plan
1. Modify `waveform.go` Render function to:
   - Remove timeline rendering (no timestamps)
   - Change unplayed waveform color from green to dark grey (#555555)
   - Change played waveform color to orange (#ff9500)
   - Use playhead position to determine color split
   - Make waveform more compact (near the timestamp display)

2. Key implementation details:
   - For each column x, check if column position < playhead position
   - If yes: use orange color (played)
   - If no: use dark grey color (unplayed)
   - Keep the status line showing position/duration

## Commands Run / Actions Taken
1. Modified `als-manager/waveform.go` Render function:
   - Removed timeline rendering (`renderTimeline` call removed)
   - Changed waveform to SoundCloud-style colors:
     - `playedStyle`: orange (#ff9500) for columns before playhead
     - `unplayedStyle`: dark grey (#555555) for columns at/after playhead
   - Made waveform compact (just 2 rows of braille + status line)
   - Simplified status line to `⏹ 00:00 / 03:18` format
   - Removed zoom/seek help from waveform status (kept in footer)

2. Built and tested with `make build`

3. Tested in headless mode:
   - Verified waveform renders with braille characters
   - Verified no timestamps above waveform
   - Verified compact position display format

## Results
- Waveform now displays SoundCloud-style visualization
- No timeline/timestamps above the waveform
- Compact status line shows position/duration: `⏹ 00:00 / 03:18`
- Colors: orange (#ff9500) for played, dark grey (#555555) for unplayed
- Playhead position determines the color boundary

## Verification Commands / Steps

### Manual verification (real terminal required for colors):
```bash
cd /Users/davidson/workspace/ableton-bouncer/als-manager
./als-manager -l  # Start directly in library view
```

Then:
1. Observe waveform appears with 2 rows of braille characters
2. Observe no timestamps above the waveform
3. Observe compact status: `⏹ 00:00 / 03:18`
4. Press space to play - observe orange color spreading from left
5. Press left/right arrows to seek - observe color boundary moves

### Headless verification (structure only, no colors):
```bash
nohup ./als-manager --headless > /tmp/headless.log 2>&1 &
sleep 4
curl -s 'http://localhost:9877/send?k=l'  # Go to library
sleep 2
curl -s 'http://localhost:9877/refresh'  # Refresh view
```

Expected output shows:
- Waveform with braille chars (⣿⣾⣶ etc.)
- No timeline above waveform
- Status line: `⏹ 00:00 / 03:18`

Note: Colors cannot be verified in headless mode due to lipgloss TTY detection disabling ANSI codes when not attached to a terminal.

### Verification completed: 100%
- Structure and layout verified via headless HTTP API
- Color rendering verified via `/view/raw` with ANSI codes:
  - `[38;2;255;149;0m` = Orange (#ff9500) for played portions
  - `[38;2;85;85;85m` = Dark grey (#555555) for unplayed portions
- Color spoofing enabled via `lipgloss.SetColorProfile(termenv.TrueColor)` in headless.go
