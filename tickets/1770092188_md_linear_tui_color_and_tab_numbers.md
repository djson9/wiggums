---
Date: 2026-02-02
Title: Linear TUI Color Patterns and Tab Numbers
Status: completed + verified
Description: Adopt similar color patterns for md linear tui as md branches tui (remove blue/purple). Add numbers to tabs (1,2,3,4) so it's clear what to press to switch.
---

## Original Request
Please adopt similar color patterns for the md linear tui as the md branches tui. I do not like the blue.

Also can we add numbers to the tabs 1,2,3,4 so that it's clear what we should press to switch to that tab

## Related Tickets
- [[tickets/1770088881_md_branches_tui_styling_consistency.md|Branches TUI styling consistency]] - Defines the color patterns we should adopt
- [[tickets/1770089667_md_linear_tui_no_color.md|Linear TUI No Color]] - Fixed color display when running via subprocess

## Execution Plan
1. **Update styles.go** - Replace purple `highlightColor` with grey-based selection style matching branches TUI:
   - Change selected item styling to use grey background (color 236) instead of purple foreground
   - Keep the same success (42/green) and error (196/red) colors
   - Update active tab to use grey/white instead of purple

2. **Update main.go View()** - Add tab numbers:
   - Change tab labels from "In Progress (5)" to "1: In Progress (5)"

3. **Update main.go issueDelegate.Render()** - Match branches TUI selection style:
   - Selected items: grey background (236) with white text (255)
   - Non-selected items: white title, dim description

4. **Build and verify** - Run the TUI and check:
   - No blue/purple colors visible
   - Tab numbers display correctly (1:, 2:, 3:, 4:)
   - Selection uses grey background like branches TUI

## Additional Context
Branches TUI uses:
- `selectedBg = lipgloss.Color("236")` for selection background
- `lipgloss.Color("255")` for selected title text
- `lipgloss.Color("250")` for selected description text
- No purple/blue anywhere

Linear TUI currently uses:
- `highlightColor = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}` (purple)
- Purple foreground for active tabs and selected items

## Commands Run / Actions Taken

1. **Updated `/Users/davidson/workspace/linear-tui/styles.go`**:
   - Removed `highlightColor` (purple `#874BFD/#7D56F4`)
   - Added `selectedBg = lipgloss.Color("236")` (grey background matching branches TUI)
   - Added `dimColor = lipgloss.Color("243")`
   - Changed `activeTabStyle` to use white (255) border and foreground instead of purple
   - Changed `selectedItemStyle` to use grey background (236) with white text (255)
   - Added `selectedDimStyle` for selected descriptions with grey background and light grey text (250)
   - Changed `inputStyle` border to white (255) instead of purple

2. **Updated `/Users/davidson/workspace/linear-tui/main.go`**:
   - Added tab numbers: `fmt.Sprintf("%d: %s (%d)", i+1, tab, count)`
   - Updated `issueDelegate.Render()` to use `selectedDimStyle` for selected item descriptions

3. **Built binary**: `go build -o linear-tui .`

## Results

All changes implemented successfully:
- **Tab numbers now visible**: "1: In Progress (5)", "2: Planned (10)", "3: Backlog (6)", "4: Done (65)"
- **No purple/blue colors**: Active tab now uses white (color 255) instead of purple
- **Selection matches branches TUI**: Selected items use grey background (236) with white text (255)
- **Tab switching works**: Pressing 1/2/3/4 switches to correct tab with white border/text

ANSI color codes observed:
- `[38;5;255m` - white foreground (active tab text)
- `[48;5;236m` - grey background (selected item)
- `[38;5;240m` - grey foreground (inactive tab borders)
- `[38;5;252m` - light grey (normal item titles)
- `[38;5;243m` - dim grey (descriptions)

## Verification Commands / Steps

```bash
# Run TUI in tmux for testing
tmux new-session -d -s linear-test -x 120 -y 35 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 5

# Capture output with ANSI codes to verify colors
tmux capture-pane -t linear-test -p -e | head -20

# Test tab switching
tmux send-keys -t linear-test '2'  # Switch to tab 2
tmux send-keys -t linear-test '3'  # Switch to tab 3

# Cleanup
tmux kill-session -t linear-test
```

**Verification completed**: 100%
- Tab numbers displayed correctly
- Colors changed from purple to white/grey
- Selection styling matches branches TUI (grey background)
- Tab switching works with number keys 1-4

## Additional Verification (2026-02-02)

Re-verified end-to-end by capturing actual TUI output with ANSI codes:

**Tab 1 (Initial view)**:
- Tab header shows: `1: In Progress (5)`, `2: Planned (10)`, `3: Backlog (6)`, `4: Done (65)`
- Active tab uses white (`[38;5;255m`), inactive tabs use grey (`[38;5;240m`)
- Selected item uses grey background (`[48;5;236m`)

**Tab switching verified**:
- Pressing '2' switched to Planned tab with correct white highlighting
- Pressing '3' switched to Backlog tab with correct white highlighting
- Content updated to show items for each respective tab

No purple/blue color codes observed - only white (255) and grey (236, 240, 243, 250, 252)
