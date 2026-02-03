---
Date: 2026-02-03
Title: Change Enter Key from Rename to Comment in Linear TUI
Status: completed + verified
Description: In md linear tui, the Enter key should be a shorthand for comment functionality instead of rename functionality.
---

## Original Request
In md linear tui, can we have enter actually be a shorthand for the comment functionality instead of rename functionality?

## Related Tickets
- [[tickets/1770090416_md_linear_tui_comments_not_showing.md|Comments Not Showing]] - Comment display functionality
- [[tickets/1770090246_md_linear_tui_cursor_and_index.md|Cursor and Index]] - Shows current help text: `enter/r: rename | c: comment`

## Plan
1. Modify `/Users/davidson/workspace/linear-tui/main.go`:
   - Line 603: Change `case "r", "enter":` to `case "r":` (rename only via 'r')
   - Line 614: Change `case "c":` to `case "c", "enter":` (comment via 'c' or Enter)
   - Line 968: Update help text from `enter/r: rename` to `r: rename | enter/c: comment`
2. Build the linear-tui binary
3. Verify by running `md linear tui` and testing Enter key behavior

## Additional Context
Current behavior:
- `Enter` or `r` → Rename mode
- `c` → Comment mode

Desired behavior:
- `r` → Rename mode
- `Enter` or `c` → Comment mode

## Commands Run / Actions Taken
1. Modified `/Users/davidson/workspace/linear-tui/main.go`:
   - Line 603: Changed `case "r", "enter":` to `case "r":`
   - Line 614: Changed `case "c":` to `case "c", "enter":`
   - Line 968: Updated help text to `r: rename | n: new | enter/c: comment`
2. Built binary: `cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .`
3. Tested in tmux session

## Results
Successfully changed Enter key binding from rename to comment functionality:
- Before: Enter/r opened rename mode, c opened comment mode
- After: r opens rename mode, Enter/c opens comment mode
- Help text updated to reflect new bindings

## Verification Commands / Steps
```bash
# Start TUI in tmux
tmux new-session -d -s linear-tui-test -x 120 -y 30 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 5

# Capture initial state - verify help text shows "r: rename | n: new | enter/c: comment"
tmux capture-pane -t linear-tui-test -p

# Test Enter key - should show "Comment:" prompt (not "Rename:")
tmux send-keys -t linear-tui-test 'Enter'
tmux capture-pane -t linear-tui-test -p

# Cancel with Escape and test 'r' key - should show "Rename:" prompt
tmux send-keys -t linear-tui-test 'Escape'
tmux send-keys -t linear-tui-test 'r'
tmux capture-pane -t linear-tui-test -p

# Cleanup
tmux kill-session -t linear-tui-test
```

**Verified:**
- Help text: Shows `r: rename | n: new | enter/c: comment`
- Enter key: Opens "Comment:" mode with empty input
- r key: Opens "Rename:" mode with current issue title pre-filled
- Escape: Cancels both modes correctly

**Verification: 100% complete** - End-to-end tested with actual TUI interaction

## Secondary Verification (2026-02-03)
Independently re-ran verification steps:
1. Started TUI in tmux - help text shows `r: rename | n: new | enter/c: comment` ✓
2. Pressed Enter - "Comment:" prompt appeared with empty input ✓
3. Pressed Escape then 'r' - "Rename:" prompt appeared with issue title pre-filled ✓
All functionality confirmed working as expected.
