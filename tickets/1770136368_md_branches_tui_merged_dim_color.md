---
Date: 2026-02-03
Title: Branches TUI - Adjust Merged PRs Dim Color
Status: completed + verified
Description: Make the dimmed shade of merged PRs in the branches TUI a little less dimmed - halfway between the current dimmed color and the active PRs.
---

## Original Request
In md branches tui. Can we have the dimmed shade of the merged PRs a little less dimmed. Maybe halfway between the current color and the active PRs?

## Related Tickets
- [[tickets/1738634000_md_branches_tui_styling_improvements.md|Styling Improvements]] - Original implementation of merged PR dimming (color 238)
- [[tickets/1770088881_md_branches_tui_styling_consistency.md|Styling Consistency]] - Made all merged items use dim color

## Technical Analysis

### Current State
- `mergedDimColor = lipgloss.Color("238")` - very dim grayscale
- `dimColor = lipgloss.Color("243")` - normal dim for status text
- Active PR titles use `lipgloss.Color("255")` - bright white

### ANSI 256 Grayscale (232-255)
- 232 = black
- 238 = current merged (very dim)
- 243 = normal dim
- 255 = white

### Calculation
Halfway between 238 (current) and 255 (active) = (238 + 255) / 2 = 246.5 ≈ 246

## Execution Plan
1. Update `mergedDimColor` from "238" to "246" in `tui/branches.go`
2. Build the CLI
3. Test TUI with merged and active PRs to verify visual difference

## Additional Context
None required - this is a simple color adjustment.

## Commands Run / Actions Taken
1. Updated `mergedDimColor` from "238" to "246" in `tui/branches.go:39`
2. Built CLI: `cd /Users/davidson/workspace/cli-middesk && go build -o md`
3. Ran TUI in tmux to capture ANSI color codes

## Results
Successfully changed merged PR dim color from 238 to 246 (halfway between old value and active PR white 255).

### Before/After Color Codes (from tmux capture)
- **Open PRs**: Title uses `[38;5;255m` (bright white), status uses `[38;5;243m` (normal dim)
- **Merged PRs**: Now use `[38;5;246m` (lighter dim) instead of `[38;5;238m` (very dim)
- **Merged state label**: Stays purple `[38;5;135m` as expected

The merged PRs are now more readable while still being visually distinct from active PRs.

## Verification Commands / Steps
```bash
# Run TUI in tmux and capture with ANSI codes
cd /Users/davidson/workspace/cli-middesk
tmux new-session -d -s tui-test -x 120 -y 30 "./md branches tui"
sleep 6
tmux capture-pane -t tui-test -p -e | head -40
tmux kill-session -t tui-test
```

### Verified
- [x] Build completes without errors
- [x] TUI displays merged PRs with color 246 (confirmed via ANSI escape sequence `[38;5;246m`)
- [x] Open PRs still use bright white (255)
- [x] Visual contrast between merged and active PRs is maintained
- [x] Merged PR state label stays purple (135)

**Verification: 100%** - End-to-end testing complete with color code verification.

## Additional Verification (2026-02-03)
Re-ran verification steps and confirmed:
- Open PRs use `[38;5;255m` (bright white)
- Merged PRs use `[38;5;246m` (lighter dim - halfway between 238 and 255)
- Merged state label uses `[38;5;135m` (purple)
- Visual contrast confirmed between open and merged PRs
