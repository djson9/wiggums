---
Date: 2026-02-02
Title: md linear tui Using Wrong API Key and Organization
Status: completed + verified
Description: The `md linear tui` command is using the shared organization's API key instead of a personal API key. It should use the personal API key from `/Users/davidson/workspace/linear-tui/.env` and run the separate `linear-tui` binary.
---

## Original Issue
With "md linear tui" it seems like we are using the wrong api key and wrong organization. We want a second linear api key in addition to the existing linear api key. In /Users/davidson/workspace/linear-tui we should have a .env with the personal api key.

## Analysis
- Current `linearTuiCmd` in `/Users/davidson/workspace/cli-middesk/cmd/linear.go` calls `lineartui.RunLinear()` which uses the shared organization API key
- The `linearOncallTuiCmd` provides a pattern: it runs an external binary at a specific path
- A separate `linear-tui` binary exists at `/Users/davidson/workspace/linear-tui/linear-tui` with its own `.env` file containing a personal API key

## Plan
1. Modify `linearTuiCmd` in `/Users/davidson/workspace/cli-middesk/cmd/linear.go` to run the external binary at `/Users/davidson/workspace/linear-tui/linear-tui` instead of calling `lineartui.RunLinear()`
2. Build the CLI
3. Verify the command works with the personal API key

## Additional Context
Related files:
- `/Users/davidson/workspace/cli-middesk/cmd/linear.go` - Main Linear CLI commands
- `/Users/davidson/workspace/linear-tui/linear-tui` - Personal Linear TUI binary
- `/Users/davidson/workspace/linear-tui/.env` - Personal API key

## Commands Run / Actions Taken
1. Modified `linearTuiCmd` in `/Users/davidson/workspace/cli-middesk/cmd/linear.go`:
   - Changed from `lineartui.RunLinear()` to running external binary at `/Users/davidson/workspace/linear-tui/linear-tui`
   - Used same pattern as `linearOncallTuiCmd` (exec.Command with stdin/stdout/stderr connected)
2. Removed unused import `lineartui "md/tui/linear"`
3. Built CLI: `go build -o md .`

## Results
Successfully modified `md linear tui` to use the personal Linear TUI binary which reads from its own `.env` file with the personal API key.

The TUI now shows personal issues (JAY-xxx from "jays-work-desk" workspace) instead of shared organization issues.

## Verification Commands / Steps
1. Verified personal API key exists in `/Users/davidson/workspace/linear-tui/.env`
2. Verified binary exists: `/Users/davidson/workspace/linear-tui/linear-tui`
3. Built CLI successfully
4. Ran `md linear tui` in tmux session:
   ```bash
   tmux new-session -d -s linear-test -x 120 -y 30 "/Users/davidson/workspace/cli-middesk/md linear tui"
   sleep 3 && tmux capture-pane -t linear-test -p
   ```
5. Observed TUI showing personal issues (JAY-190, JAY-185, JAY-182, etc.) from "jays-work-desk" workspace
6. Cleaned up tmux session

**Verification: 100%** - Full end-to-end verification completed. The TUI now displays personal Linear issues using the personal API key.

## Additional Verification Notes (2026-02-02)
- Confirmed code change in `/Users/davidson/workspace/cli-middesk/cmd/linear.go:1971-2002` runs external binary
- Confirmed binary exists: `/Users/davidson/workspace/linear-tui/linear-tui` (10MB, executable)
- Confirmed personal `.env` file exists at `/Users/davidson/workspace/linear-tui/.env`
- Original verification showed personal issues (JAY-xxx from "jays-work-desk") - proper end-to-end test
