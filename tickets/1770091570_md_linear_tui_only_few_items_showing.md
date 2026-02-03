---
Date: 2026-02-02
Title: md linear tui only shows few items even on tall terminals
Status: completed + verified
Description: In md linear tui, only a few items show up even when the terminal is very tall, leaving significant whitespace at the bottom.
---

## Original Request
In md linear tui, even when the terminal is very tall, only a few items show up.

```
╭───────────────────╮╭────────────────╮╭───────────────╮╭─────────────╮
│  In Progress (5)  ││  Planned (10)  ││  Backlog (6)  ││  Done (65)  │
╰───────────────────╯╰────────────────╯╰───────────────╯╰─────────────╯


 1. JAY-190: Rosanna PR review
    https://linear.app/jays-work-desk/issue/JAY-190/rosanna-pr-review
      [Feb 2 8:35pm]: This is a longer comment to test the word wrapping

 2. JAY-185: Geocoder revert reverted changes in Middesk
    https://linear.app/jays-work-desk/issue/JAY-185/geocoder-revert-rev…
      [Feb 2 5:26pm]: Opened PR for adding to middesk

 3. JAY-182: Sean PR review
    https://linear.app/jays-work-desk/issue/JAY-182/sean-pr-review
 4. JAY-178: What's going on with FL retrievals?
    https://linear.app/jays-work-desk/issue/JAY-178/whats-going-on-with…

  ••
 Item 1 of 5

1-4: tabs | j/k: nav | enter/r: rename | n: new | c: comment | space: cut | v: paste | o: open | alt+↑/↓: reorder | q: quit




(lots of whitespace below)


```

I think there is some issue with environment variable terminal height or something along those lines

## Related Tickets
- [[tickets/1770084000_md_branches_tui_list_items_not_extending.md|Branches TUI list items not extending]] - Same issue fixed in branches TUI
- [[tickets/1770090416_md_linear_tui_comments_not_showing.md|Linear TUI comments not showing]] - Terminal size detection at startup was added

## Root Cause Analysis
Comparing to the branches TUI fix, the linear-tui had three issues:

1. **Terminal detection order**: Code tried `os.Stdout.Fd()` but should try `os.Stdin.Fd()` first (preferred for bubbletea apps).

2. **`l.Paginator.PerPage` not set at startup**: The list paginator was never explicitly set with items per page. The bubbles list component uses `WindowSizeMsg` to configure it, but `WindowSizeMsg` may not be received in tmux/iTerm contexts.

3. **Filtering enabled causing pagination behavior**: The list had `SetFilteringEnabled(true)` which caused the list to use page-by-page navigation instead of continuous scrolling. Setting `SetFilteringEnabled(false)` to match branches TUI fixed this.

4. **Pagination dots visible**: `SetShowPagination(false)` was not called, leaving pagination dots visible.

## Execution Plan
1. Modify `/Users/davidson/workspace/linear-tui/main.go`:
   - Update `newModel()` to try `os.Stdin.Fd()` first for terminal size detection
   - Add `l.Paginator.PerPage = listHeight / delegateHeight` for each list at startup
   - Add `l.SetShowPagination(false)` to hide pagination dots
   - Set `l.SetFilteringEnabled(false)` to enable continuous scrolling
2. Rebuild the linear-tui binary
3. Test with tmux at different terminal heights
4. Verify all items show and no whitespace at bottom

## Commands Run / Actions Taken

1. **Modified `/Users/davidson/workspace/linear-tui/main.go`**:
   - Fixed terminal detection to try stdin first, then stdout:
     ```go
     if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil && w > 0 && h > 0 {
         width, height = w, h
     } else if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && h > 0 {
         width, height = w, h
     }
     ```
   - Added paginator PerPage setting in list initialization loop:
     ```go
     l.SetShowPagination(false) // Hide pagination dots
     l.SetFilteringEnabled(false) // Enable continuous scrolling
     l.Paginator.PerPage = listHeight / delegateHeight
     ```

2. **Built the binary**:
   ```bash
   cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .
   ```

## Results
The linear TUI now correctly displays all items that fit in the terminal height:

**Before (height 50, Planned tab with 10 items):**
- Only 4-9 items visible
- Pagination dots shown ("••")
- Page-by-page navigation

**After (height 50, Planned tab with 10 items):**
- All 10 items visible
- No pagination dots
- Continuous scrolling when navigating past visible items

## Verification Commands / Steps

### Test 1: Full height terminal (50 lines) - PASSED
```bash
tmux new-session -d -s linear-test -x 120 -y 50 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 7 && tmux send-keys -t linear-test '2' && sleep 1 && tmux capture-pane -t linear-test -p
```
Result: All 10 items in "Planned" tab visible (items 1-10 shown lines 5-25)

### Test 2: Medium height terminal (30 lines) - PASSED
```bash
tmux new-session -d -s linear-test -x 120 -y 30 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 7 && tmux send-keys -t linear-test '2' && sleep 1 && tmux capture-pane -t linear-test -p
```
Result: 6 items visible (fitting the smaller terminal), scrolling works to reveal items 7-10

### Test 3: Scrolling in Done tab (65 items) - PASSED
```bash
tmux send-keys -t linear-test '4'  # Switch to Done tab
for i in {1..20}; do tmux send-keys -t linear-test 'j'; sleep 0.1; done
tmux capture-pane -t linear-test -p
```
Result: Scrolling works continuously, showing items 21-30 after scrolling down

### Test 4: Via md CLI command - PASSED
```bash
tmux new-session -d -s linear-test -x 120 -y 40 "/Users/davidson/workspace/cli-middesk/md linear tui"
sleep 8 && tmux capture-pane -t linear-test -p
```
Result: All 5 items in "In Progress" visible with comments shown for items with them

**Verification: 100% complete**
- Initial view shows all items that fit: VERIFIED
- Continuous scrolling works: VERIFIED
- Works at different terminal heights: VERIFIED
- Works via md CLI wrapper: VERIFIED
- No pagination dots: VERIFIED

## Bugs Encountered
None - the fix was successful and all functionality works as expected.

## Key Learning (for shortcuts.md)
**Critical fix for bubbles/list pagination vs scrolling:** Setting `SetFilteringEnabled(false)` is required for continuous scrolling behavior. When filtering is enabled, the list uses page-by-page navigation instead of continuous scrolling, which causes the "few items showing" issue even when PerPage is set correctly.

## Additional Verification (2026-02-02)
Independent verification confirmed:
- ✅ "In Progress" tab: All 5 items visible
- ✅ "Planned" tab: All 10 items visible (key fix - was only showing 4-9 before)
- ✅ "Done" tab: Continuous scrolling works (navigated to items 15-21 using j-key)
- ✅ No pagination dots visible
