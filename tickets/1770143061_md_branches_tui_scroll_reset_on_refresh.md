---
Date: 2026-02-03
Title: md branches TUI Scroll Position Resets on Auto-Refresh
Status: completed + verified
Description: When scrolled down to view branches beyond the first page in md branches tui, the view resets to the top after the 30-second auto-refresh interval.
---

## Original Request
I notice on md branches tui, when I'm scrolled down to the next page, after 30s or so it refreshes back up to the top of the screen

## Related Tickets
- [[tickets/1770138698_md_branches_tui_realtime_check_timer.md|md branches TUI Running Status Check Timer]] - Implemented the 30-second API tick that triggers the refresh

## Root Cause Analysis
In `/Users/davidson/workspace/cli-middesk/tui/branches.go`, the `branchesLoadedMsg` handler (lines 479-490):

```go
case branchesLoadedMsg:
    m.loading = false
    m.lastRefresh = time.Now()
    if msg.err != nil {
        m.err = msg.err
        return m, nil
    }
    m.branches = msg.branches
    m.refreshList()
    m.list.ResetFilter()
    m.list.Select(0)  // <-- BUG: Always resets to first item!
    return m, nil
```

Every 30 seconds, `tickMsg` triggers `fetchBranchesCmd`, which returns `branchesLoadedMsg`, which calls `m.list.Select(0)` - resetting the selection to the first item regardless of where the user was scrolled.

## Solution
1. Save the current selection index before refreshing the list
2. After refreshing, restore the selection to the saved index (clamped to valid bounds)
3. Only reset to 0 on initial load (when `m.branches` is empty)

## Execution Plan
1. In the `branchesLoadedMsg` handler:
   - Save `currentIndex := m.list.Index()` before refreshing
   - Check if this is an initial load (`len(m.branches) == 0`)
   - After `refreshList()`, restore selection: if initial load → `Select(0)`, else → `Select(min(currentIndex, len-1))`
2. Build and verify with `go build ./...`
3. Test manually in TUI by scrolling down and waiting for refresh

## Commands Run / Actions Taken
1. Edited `/Users/davidson/workspace/cli-middesk/tui/branches.go` lines 479-490 to preserve scroll position:
   - Save `currentIndex := m.list.Index()` before refreshing
   - Check `isInitialLoad := len(m.branches) == 0`
   - After refresh: initial load → `Select(0)`, otherwise → `Select(min(currentIndex, len-1))`

2. Built with `go build -o md .` - SUCCESS

3. Tested in tmux:
   ```bash
   tmux new-session -d -s branches-test -x 120 -y 35 "./md branches tui --limit 15"
   ```

## Results
- Scroll position is now preserved during auto-refresh
- Initial load still starts at position 0 (as expected)
- If list shrinks during refresh, position clamps to last valid item

## Verification Commands / Steps

### Before Fix (expected behavior)
- Start TUI: `md branches tui`
- Navigate down with j key (8 times) to reach item 9
- Wait ~30 seconds for auto-refresh
- **BUG**: Position resets to Item 1

### After Fix (actual test)
1. Started TUI in tmux with `--limit 15`
2. Navigated to Item 9 of 15 (tick:22, last refresh: 13:25:07)
3. Waited 35 seconds for auto-refresh
4. **VERIFIED**: Position still at Item 9 of 15 (tick:64, last refresh: 13:26:06)
5. Verified visible content shows items 9-13 (not items 1-5)

**Verification completed: 100%**
- [x] Code compiles successfully
- [x] Scroll position preserved through refresh (tested end-to-end)
- [x] Initial load still goes to position 0
- [x] Verified visible content matches expected position
