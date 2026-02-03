---
Date: 2026-02-02
Title: Linear TUI cursor position and index numbers
Status: completed + verified
Description: Add cursor position display ("Item X of Y") and index numbers (1., 2., 3.) to md linear tui, matching the styling of md branches tui.
---
Can we have md linear tui show cursor position like we do for md branches tui.

Also please add index numbers like we do for md branches tui

---

## Plan

### Related Tickets
- [[tickets/1738634000_md_branches_tui_styling_improvements.md|Branches TUI styling improvements]] - Original implementation of index numbers and cursor position for branches TUI
- [[tickets/1770089902_md_linear_tui_navigation_corruption.md|Linear TUI navigation corruption]] - Previous fix added ANSI clear codes to linear TUI

### Implementation Steps

1. **Add cursor position to View()** - Add "Item X of Y" to status/help line in `/Users/davidson/workspace/linear-tui/main.go`
   - Get current index from `m.lists[m.activeTab].Index()`
   - Get total from `len(m.issues[tabName])`
   - Display in status area at bottom

2. **Create custom delegate with index numbers** - Similar to `branchDelegate` in branches.go
   - Create `issueDelegate` struct with Height/Spacing/Update/Render methods
   - In Render(), prefix each item with " N. " (2-digit padded format)
   - Replace `list.NewDefaultDelegate()` with custom delegate

### Commands Run / Actions Taken

1. Added `io` import to main.go for Fprintf in delegate
2. Created `issueDelegate` struct with Render method that displays 2-digit padded index (` 1. `, ` 2. `, etc.)
3. Updated `newModel()` to use `newIssueDelegate()` instead of `list.NewDefaultDelegate()`
4. Updated `WindowSizeMsg` handler to use `newIssueDelegate()` instead of `list.NewDefaultDelegate()`
5. Added cursor position display ("Item X of Y") to View() function, displayed in status line before help text
6. Built the binary: `cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .`

### Results

Successfully implemented both features in `/Users/davidson/workspace/linear-tui/main.go`:

**Index numbers**: Each list item now shows a 2-digit padded index prefix:
```
 1. JAY-190: Rosanna PR review
    https://linear.app/jays-work-desk/issue/JAY-190/rosanna-pr-review
 2. JAY-185: Geocoder revert reverted changes in Middesk
    https://linear.app/jays-work-desk/issue/JAY-185/geocoder-revert-reverted-changes-in-middesk
```

**Cursor position**: Status line at bottom now shows "Item X of Y":
```
 Item 3 of 5
1-4: tabs | j/k: nav | enter/r: rename | n: new | c: comment | space: cut | v: paste | o: open | alt+↑/↓: reorder | q: q
```

The selected item's index is highlighted in bold purple, while non-selected item indices are dimmed.

### Verification Commands / Steps

1. **Built successfully**: `go build -o linear-tui .` - no errors
2. **Tested in tmux**: `tmux new-session -d -s linear-test -x 120 -y 30 "/Users/davidson/workspace/linear-tui/linear-tui"`
3. **Verified index numbers visible**: Captured output shows " 1. ", " 2. ", " 3. ", etc. prefixing each item
4. **Verified cursor position before navigation**: Shows "Item 1 of 5" when on first item
5. **Verified cursor position after navigation** (pressed j twice): Shows "Item 3 of 5" - position updates correctly
6. **Verified tab switching**: Switched to Planned tab (key 2), shows "Item 1 of 10" - count updates correctly for different tabs
7. **Verified ANSI highlighting**: Captured with `-e` flag shows selected item index is bold purple `[1m[38;2;125;86;243m 1.`

**Verification: 100% complete** - All features working end-to-end

### Additional Verification (2026-02-02)

Independent verification confirmed all features work:
- Ran `linear-tui` in tmux, captured output showing index numbers (1., 2., 3., etc.)
- Initial view showed "Item 1 of 5" on In Progress tab
- After pressing `j` twice, cursor position updated to "Item 3 of 5"
- Switched to Planned tab (key 2), count correctly updated to "Item 1 of 10"
- All end-to-end tests pass
