---
Date: 2026-02-02
Title: Linear TUI Help Menu Corrupted on Scroll
Status: completed + verified
Description: When scrolling to the next page in md linear tui, the help menu at the bottom gets broken up and duplicated. Text artifacts appear showing the help text repeated/overlapping.
---

## Original Report
In md linear tui, when scrolling to the next page, the help menu gets broken up like this

```
╭───────────────────╮╭────────────────╮╭───────────────╮╭─────────────╮
│  In Progress (5)  ││  Planned (10)  ││  Backlog (6)  ││  Done (65)  │
╰───────────────────╯╰────────────────╯╰───────────────╯╰─────────────╯


 5. JAY-140: Iterate on FL submission
    https://linear.app/jays-work-desk/issue/JAY-140/iterate-on-fl-submi…




  ••
 Item 5 of 5
                                                                                                                                                                                                                     1-4: tabs | j/k: nav | enter/r: rename | n: new | c: comment | space: cut | v: paste | o: open | alt+↑/↓: reorder | q: quit nav | enter/r: rename | n: new | c: comment | space: cut | v: paste | o: open | alt+↑/↓: r
```

## Related Tickets
- [[tickets/1770089902_md_linear_tui_navigation_corruption.md|Linear TUI Navigation Corruption]] - Same root cause (missing clear-to-EOL codes) but for list items
- [[tickets/1770084000_md_branches_tui_list_items_not_extending.md|Branches TUI Items Not Extending]] - Similar rendering artifact issues

## Root Cause Analysis
The help line and status line at the bottom of the View() function do not have ANSI clear-to-end-of-line codes (`\033[K`). When the terminal redraws (e.g., during pagination/scrolling), the old content isn't cleared, causing visual artifacts.

Looking at `/Users/davidson/workspace/linear-tui/main.go`:
- Lines 877-884: List view already has clearEOL codes
- Lines 887-900: Status line - MISSING clearEOL
- Lines 903-915: Help line - MISSING clearEOL

## Execution Plan
1. ✅ Move `clearEOL` definition outside the list block so it can be reused
2. ✅ Add `\033[K` (clearEOL) after the status line rendering
3. ✅ Add `\033[K` (clearEOL) after the help line rendering
4. ✅ Rebuild the linear-tui binary
5. ✅ Test by running `md linear tui` and scrolling through pages

## Additional Context
None gathered.

## Commands Run / Actions Taken
1. Edited `/Users/davidson/workspace/linear-tui/main.go`:
   - Moved `clearEOL := "\033[K"` to before the list block (line 878)
   - Added `+ clearEOL` after status line rendering (line 897, 900)
   - Added `+ clearEOL` after help line rendering (line 915)
2. Built binary: `cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .`

## Results
The fix successfully prevents help menu corruption during scrolling. The ANSI clear-to-end-of-line codes now properly clear any residual content when the terminal redraws.

## Verification Commands / Steps
```bash
# Run TUI in tmux for testing
tmux new-session -d -s tui-test -x 120 -y 30 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 8

# Test initial state
tmux capture-pane -t tui-test -p

# Scroll down through items (j = down)
for i in $(seq 1 20); do tmux send-keys -t tui-test 'j'; sleep 0.1; done
tmux capture-pane -t tui-test -p  # Verify no corruption

# Continue scrolling to stress test pagination
for i in $(seq 1 30); do tmux send-keys -t tui-test 'j'; sleep 0.1; done
tmux capture-pane -t tui-test -p  # Verify no corruption at item 51 of 65

# Scroll back up
for i in $(seq 1 50); do tmux send-keys -t tui-test 'k'; sleep 0.1; done
tmux capture-pane -t tui-test -p  # Verify no corruption at item 1 of 65

# Clean up
tmux send-keys -t tui-test 'q'
tmux kill-session -t tui-test
```

### Test Results
- ✅ Initial state: Help menu renders cleanly
- ✅ After scrolling to item 21 of 65: No corruption
- ✅ After scrolling to item 51 of 65: No corruption
- ✅ After scrolling back to item 1 of 65: No corruption

**Verification: 100% complete** - End-to-end testing confirmed the fix works across multiple pagination scenarios.

### Additional Verification (2026-02-02)
Independently verified by running live TUI test:
- Confirmed code changes in `/Users/davidson/workspace/linear-tui/main.go` lines 877, 898, 901, 917 with `clearEOL` codes
- Binary rebuilt today (Feb 2 22:59)
- Ran TUI in tmux, switched to Done tab (65 items)
- Scrolled to item 21, item 51, and back to item 1
- Help line remained clean throughout all pagination scenarios - no duplication or artifacts
