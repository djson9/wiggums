---
Date: 2026-02-02
Title: md linear tui No Color Display
Status: completed + verified
Description: The `md linear tui` command was not displaying colors. The TUI rendered correctly with box-drawing characters but all text was plain without ANSI color codes.
---

## Original Issue
For some reason md linear tui has no color anymore. Are you able to reproduce? Please fix if you can reproduce.

## Analysis
The issue was confirmed reproducible. Investigation revealed:

1. `md linear tui` runs an external binary at `/Users/davidson/workspace/linear-tui/linear-tui` via `exec.Command`
2. `md branches tui` (which has colors) runs TUI code directly within the same process
3. When the linear-tui binary is run directly from the shell, colors work correctly
4. When run via `exec.Command`, the child process fails to detect terminal color capability

The root cause is that lipgloss/termenv's color detection doesn't work correctly when running as a subprocess via exec.Command, even with stdin/stdout/stderr properly connected. The child process doesn't receive proper TTY information.

## Related Tickets
- [[tickets/1738549200_md_linear_tui_wrong_api_key.md|md linear tui Using Wrong API Key]] - Changed linearTuiCmd to use external binary
- [[tickets/1738634000_md_branches_tui_styling_improvements.md|md branches tui styling improvements]] - Recent TUI styling work
- [[tickets/1770088881_md_branches_tui_styling_consistency.md|md branches tui styling consistency]] - Branches TUI styling (has colors)

## Plan
1. Confirm reproduction by running `md linear tui` in tmux and capturing ANSI codes
2. Compare with `md branches tui` which has colors
3. Test running linear-tui binary directly vs via exec.Command
4. Fix by adding CLICOLOR_FORCE=1 environment variable to subprocess

## Additional Context
The issue affects any TUI binary run via exec.Command in the md CLI. Both `linearTuiCmd` and `linearOncallTuiCmd` use this pattern.

## Commands Run / Actions Taken
1. Reproduced the issue by running `md linear tui` in tmux session:
   ```bash
   tmux send-keys -t test1 "/Users/davidson/workspace/cli-middesk/md linear tui" Enter
   tmux capture-pane -t test1 -p -e | cat -v  # No ANSI color codes visible
   ```

2. Verified `md branches tui` DOES have colors (ANSI codes like `^[[38;5;250m`)

3. Tested running linear-tui binary directly (has colors):
   ```bash
   /Users/davidson/workspace/linear-tui/linear-tui  # Colors work!
   ```

4. Tested with CLICOLOR_FORCE=1 environment variable:
   ```bash
   CLICOLOR_FORCE=1 /Users/davidson/workspace/cli-middesk/md linear tui  # Colors work!
   ```

5. Modified `/Users/davidson/workspace/cli-middesk/cmd/linear.go` to add environment variable to child process:
   ```go
   c.Env = append(os.Environ(), "CLICOLOR_FORCE=1")
   ```

6. Applied fix to both `linearTuiCmd` and `linearOncallTuiCmd`

7. Rebuilt CLI: `go build -o md .`

## Results
Successfully fixed the color issue. The `md linear tui` command now displays colors correctly with purple highlights, grey inactive tabs, and colored text for selected items.

## Verification Commands / Steps
1. Ran `md linear tui` in tmux session test1
2. Captured with ANSI codes: `tmux capture-pane -t test1 -p -e | cat -v`
3. Verified ANSI escape codes present:
   - `^[[94m` - bright blue/purple (active tab border)
   - `^[[90m` - dark grey (inactive elements)
   - `^[[95m` - bright magenta (selected item)
   - `^[[37m` - white (normal item text)
   - `^[[1m` - bold
   - `^[[0m` - reset

4. Visual verification: Tab borders and selected items now show distinct colors

**Verification: 100%** - Full end-to-end verification completed. Colors are now displayed correctly in `md linear tui`.

## Additional Verification (2026-02-02)
Independently verified the fix:
1. Confirmed code change present in `cmd/linear.go` at lines 1998 and 2028
2. Ran `md linear tui` in tmux session test1
3. Captured output with `tmux capture-pane -t test1 -p -e | cat -v`
4. Confirmed ANSI codes present: `^[[94m` (blue), `^[[90m` (grey), `^[[95m` (magenta), `^[[37m` (white), `^[[1m` (bold)
5. TUI displayed "In Progress (5)", "Planned (10)", "Backlog (6)", "Done (65)" tabs with colors and styling
