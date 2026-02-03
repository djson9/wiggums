---
Date: 2026-02-02
Title: Linear TUI comments not showing
Status: completed + verified
Description: Investigate whether md linear tui is properly displaying comments - should show top 2 recent comments with timestamp and content.
---
I notice md linear tui does not show comments. Can you reproduce? It should be showing top 2 recent comments with timestamp and comment content.

## Related Tickets
- [[tickets/1770089902_md_linear_tui_navigation_corruption.md|Navigation corruption fix]] - Fixed rendering artifacts
- [[tickets/1770090246_md_linear_tui_cursor_and_index.md|Cursor and index display]] - Added position info

## Root Cause Analysis
The issue was two-fold:

1. **Terminal size thresholds too high**: The delegate height (which controls how many lines per item) was set based on terminal height with thresholds that excluded common terminal sizes:
   - Original: `height >= 40` for 2 comments, `height >= 30` for 1 comment, `height < 30` for NO comments
   - This meant terminals with 24-29 rows never saw comments

2. **No startup terminal size detection**: The code relied solely on `tea.WindowSizeMsg` to set delegate height, but this message isn't always received reliably in all terminal contexts (especially when running via `exec.Command` through the CLI)

## Solution
Modified `/Users/davidson/workspace/linear-tui/main.go`:

1. Added `golang.org/x/term` import for terminal size detection
2. Updated `newModel()` to detect terminal size at startup using:
   - `LINES`/`COLUMNS` environment variables (works in tmux)
   - `term.GetSize()` as fallback
3. Set delegate height and list dimensions at startup instead of waiting for `WindowSizeMsg`
4. Lowered thresholds for comment display:
   - `height >= 35`: 4 lines (URL + 2 comments)
   - `height >= 24`: 3 lines (URL + 1 comment)
   - `height < 24`: 2 lines (URL only)

## Additional Context
The code structure for comments was already in place:
- GraphQL query fetches `comments(first: 2, orderBy: createdAt)`
- `IssueItem.Description()` in `issue.go` formats comments with timestamps
- `issueDelegate.Render()` displays description lines based on delegate height

## Commands Run / Actions Taken
1. Read main.go, issue.go, api.go to understand code structure
2. Tested at different terminal heights using tmux:
   - Height 40: Comments showed (before fix)
   - Height 25: No comments (before fix)
3. Added `strconv` and `golang.org/x/term` imports
4. Updated `newModel()` with terminal size detection at startup
5. Updated thresholds in both `newModel()` and `WindowSizeMsg` handler
6. Ran `go get golang.org/x/term && go mod tidy`
7. Built binary with `go build -o linear-tui .`
8. Verified fix at multiple terminal heights

## Results
Comments now display correctly in terminals with height >= 24 rows. The fix ensures:
- 1 comment shows for terminals 24-34 rows
- 2 comments show for terminals 35+ rows
- Terminal size is detected at startup, not just via WindowSizeMsg

## Verification Commands / Steps
```bash
# Test at height 25 (previously failed, now shows 1 comment)
tmux new-session -d -s test -x 120 -y 25 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 5 && tmux capture-pane -t test -p && tmux kill-session -t test

# Test at height 40 (shows 2 comments)
tmux new-session -d -s test -x 120 -y 40 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 5 && tmux capture-pane -t test -p && tmux kill-session -t test

# Test via md CLI at height 28
tmux new-session -d -s test -x 120 -y 28 "/Users/davidson/workspace/cli-middesk/md linear tui"
sleep 5 && tmux capture-pane -t test -p && tmux kill-session -t test
```

**Before fix (height 25):**
- Only URL shown, no comments

**After fix (height 25):**
- URL + 1 comment with timestamp shown (e.g., `[Feb 2 8:35pm]: This is a longer comment...`)

**Verification: 100% complete**
- Tested height 20: No comments (expected, below 24 threshold)
- Tested height 25: 1 comment showing (fixed)
- Tested height 28: 1 comment showing via `md linear tui` (fixed)
- Tested height 40: 2 comments showing (working as expected)

## Additional Verification (2026-02-02)
Independent verification performed by running the actual TUI commands.

**Test 1: Direct binary at height 25**
```
tmux new-session -d -s test -x 120 -y 25 "/Users/davidson/workspace/linear-tui/linear-tui"
```
Result: Comments visible - `[Feb 2 8:35pm]: This is a longer comment to test the word wrapping functionality...`

**Test 2: Via md CLI at height 28**
```
tmux new-session -d -s test -x 120 -y 28 "/Users/davidson/workspace/cli-middesk/md linear tui"
```
Result: Comments visible - Both JAY-190 and JAY-185 showed timestamps and comment text.

**Conclusion**: Fix verified working end-to-end through both direct binary and CLI invocation.
