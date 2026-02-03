---
Date: 2026-02-03
Title: Linear TUI Dim Comments Less Dim and Add Spacing Between Entries
Status: completed + verified
Description: In md linear tui, make the dimmed comments a little less dim, and add one new line between all entries in the list.
---

In md linear tui, can we also make the dimmed comments a little less dim.

And then add one new line between all of the entries in the list.

## Related Tickets
- [[tickets/1770136368_md_branches_tui_merged_dim_color.md|Branches TUI Merged Dim Color]] - Similar fix for branches TUI, changed color from 238 to 246
- [[tickets/1770090416_md_linear_tui_comments_not_showing.md|Linear TUI Comments Not Showing]] - Previous fix that enabled comments display

## Additional Context
The linear-tui is located at `/Users/davidson/workspace/linear-tui/`.

Current state (before fix):
- `dimColor` in `styles.go` was set to "243" (fairly dim grey)
- `dimStyle` in `styles.go` used color "243"
- `spacing` in `newIssueDelegate()` was set to 0 (no blank lines between items)

## Execution Plan
1. Change `dimColor` from "243" to "246" in `styles.go` line 8
2. Change `dimStyle` foreground from "243" to "246" in `styles.go` line 49
3. Change `spacing` from 0 to 1 in `main.go` line 58
4. Build the linear-tui binary
5. Verify changes using tmux capture

## Commands Run / Actions Taken
1. Explored related tickets to understand prior work on linear-tui styling
2. Read `/Users/davidson/workspace/linear-tui/styles.go` and `/Users/davidson/workspace/linear-tui/main.go`
3. Edited `styles.go` line 8: Changed `dimColor = lipgloss.Color("243")` to `lipgloss.Color("246")`
4. Edited `styles.go` lines 48-49: Changed `dimStyle` foreground from "243" to "246"
5. Edited `main.go` line 58: Changed `spacing: 0` to `spacing: 1` with comment `// one blank line between entries`
6. Built binary: `cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .`
7. Verified in tmux session

## Results
Both requested changes implemented:
1. **Dimmed comments less dim**: Changed from color 243 to 246 (matches the fix done for branches TUI in ticket 1770136368)
2. **Spacing between entries**: Added `spacing: 1` in the issue delegate, which adds one blank line between each list entry

Files modified:
- `/Users/davidson/workspace/linear-tui/styles.go` (lines 8, 49)
- `/Users/davidson/workspace/linear-tui/main.go` (line 58)

## Verification Commands / Steps
```bash
# Start TUI in tmux
tmux new-session -d -s linear-test -x 120 -y 40 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 5

# Capture plain text to verify spacing
tmux capture-pane -t linear-test -p

# Capture with ANSI codes to verify color changes
tmux capture-pane -t linear-test -p -e | cat -v

# Cleanup
tmux kill-session -t linear-test
```

### Verification Results
1. **Spacing verified**: Plain text capture showed blank lines between entries (item 1, blank, item 2, blank, item 3, etc.)
2. **Color verified**: ANSI capture showed `^[[38;5;246m` for non-selected item descriptions/comments (previously would have been 243)
   - Non-selected items: `^[[38;5;246m` (color 246 - dim but visible)
   - Selected items: `^[[38;5;250m` (color 250 - brighter for selectedDimStyle)

**Verification: 100% complete** - Both changes verified end-to-end via TUI capture.

## Additional Verification (2026-02-03)
Independent re-verification confirmed:
- Code changes present in `styles.go` (lines 8, 49: color 246) and `main.go` (line 58: spacing 1)
- TUI launched and captured via tmux
- Plain text output shows blank lines between all 5 entries
- ANSI capture confirms `^[[38;5;246m` for non-selected dimmed URLs/comments
- Selected items correctly use `^[[38;5;250m` for selectedDimStyle
