# Shortcuts - Iteration Learnings

## Investigating Middesk PRs and Incidents
- Use `gh pr view <PR#> --repo middesk/middesk` to view PR summary
- Use `gh pr diff <PR#> --repo middesk/middesk` to see code changes
- Use `gh pr view <PR#> --repo middesk/middesk --comments` for discussion context
- Use `git log --oneline -20 -- <filepath>` in `/Users/davidson/workspace/middesk` to trace related commits
- Use `git show <commit>` to see full diffs of specific commits

## PR Review Comments
- Use `md pr comments middesk <PR#>` for middesk/middesk repo
- Use `md pr comments middesk/geocoder <PR#>` for other repos (full org/repo format)
- Use `gh pr view <PR#> --repo <org/repo> --json reviews,comments,state,reviewDecision` for full PR status
- Use `gh pr checks <PR#> --repo <org/repo>` to check CI status

## Finding PRs by Branch Name
- Use `gh pr list --repo <org/repo> --head <branch-name> --json number,title,url`
- Use `gh pr list --repo <org/repo> --search "<keywords>" --json number,title,url,headRefName` for keyword search

## Viewing Remote File Contents (without cloning)
- Use `gh api "repos/<org>/<repo>/contents/<path>" --jq '.content' | base64 -d` for main branch
- Use `gh api "repos/<org>/<repo>/contents/<path>?ref=<branch>" --jq '.content' | base64 -d` for specific branch
- Pipe to `| sed -n '<start>,<end>p'` to view specific line ranges

## Schema/Migration Investigation Pattern
When investigating schema issues:
1. Read the model file for schema annotation (top of file shows columns)
2. Grep for existing migrations: `create_table|add_column|change_table.*<table_name>`
3. Look at similar recent migrations for the correct format/Rails version

## Creating Migrations in Middesk

**Preferred method:** `cd /Users/davidson/workspace/middesk && bin/rails generate migration MigrationName column:type`

**If Ruby version mismatch:** Manually create migration file:
1. Get timestamp: `date +"%Y%m%d%H%M%S"`
2. Create file: `db/migrate/TIMESTAMP_snake_case_name.rb`
3. Use `ActiveRecord::Migration[8.0]` (check recent migrations for current version)
4. Follow existing patterns (see `db/migrate/` for examples)

**About structure.sql:** Auto-updated when migrations run (`config.active_record.schema_format = :sql`). No manual edits needed.

**Ruby version requirement:** Use `nix develop` for correct Ruby version:
```bash
cd /Users/davidson/workspace/middesk
nix develop --command bin/rails db:migrate
nix develop --command bin/rails db:migrate:status
nix develop --command bundle install  # if gems missing
```

## Investigating Skyvern/Agent Tasks
Agent commands require a running middesk console pod. Create one first:
```bash
md kube pod new middesk          # Creates pod, shows pod name
md kube pod ls                   # List running pods
md kube pod kill <pod-name> -n production  # Cleanup when done
```

## Running Commands in Staging Environment
Unlike production (`md kube pod new middesk`), staging requires manual pod creation via `make console-staging`.
The pod only exists while the console is running (EXIT trap deletes it).

**Start staging console (keep running in Terminal A):**
```bash
cd /Users/davidson/workspace/middesk
nix develop --command make console-staging
```

**Run commands in staging (Terminal B):**
```bash
md middesk run "1+1" --staging
md middesk run "User.count" --staging
md middesk run "Rails.env" --staging  # Verify: should return "staging"
```

**Alternative: tmux workflow for non-interactive use:**
```bash
# Start
tmux new-session -d -s staging-console -c ~/workspace/middesk
tmux send-keys -t staging-console "nix develop --command make console-staging" Enter
sleep 15  # Wait for pod

# Use
md middesk run "..." --staging

# Cleanup
kubectl get pods -n staging -l role=console-user  # Find pod name
kubectl delete pod <pod-name> -n staging
tmux kill-session -t staging-console
```

**Mutual exclusivity:** `--prod` and `--staging` cannot be used together.

**Viewing specific agent tasks:**
```bash
md agent task show <task-uuid>
```

**Skyvern workflow reports (requires pod):**
```bash
md agent skyvern retrieval fl_department_of_revenue --limit-days 14
md agent skyvern submission ca_employment_development_department --limit-days 7
```

**Viewing Skyvern run details:**
```bash
md skyvern runs <run_id>         # Get full output including extracted_information
md skyvern workflows             # List all workflows
```

**Linear issue investigation:**
```bash
md linear show AGSUP-2685        # Get full issue details as JSON
md linear oncall ls              # List oncall queue
```

## Key Skyvern Files in Middesk
- Base retrieval logic: `/Users/davidson/workspace/middesk/app/jobs/agent/skyvern_workflows/base_retrieval.rb`
- Agency configs: `/Users/davidson/workspace/middesk/app/jobs/agent/skyvern_workflows/<agency>_retrieval_config.rb`
- Constants (account IDs): `/Users/davidson/workspace/middesk/app/lib/agent/constants.rb`
- The `account_status` (success/processing/remediation/duplicate) comes from Skyvern extraction, not Ruby code

## Debugging TUI Applications (bubbletea/bubbles)

**Running TUI in tmux for debugging:**
```bash
tmux new-session -d -s tui-test -x 120 -y 30 "md branches tui"
sleep 7  # Wait for data to load
tmux capture-pane -t tui-test -p  # Capture plain text
tmux capture-pane -t tui-test -p -e | cat -v  # Capture with ANSI codes visible
tmux send-keys -t tui-test 'j'  # Send keystrokes
tmux kill-session -t tui-test  # Cleanup
```

**Common TUI rendering issues:**
- **Rendering artifacts when scrolling:** bubbles/list doesn't clear full lines. Fix by adding `\033[K` (clear-to-EOL) to ALL lines in View() - not just list items, but also status lines, help lines, and any other rendered content
- **ANSI codes breaking width calculation:** If Title()/Description() return pre-styled text with ANSI codes, the list's width calculation may be incorrect. Return plain text and let the delegate style it
- **Items not extending to full width:** Use lipgloss.Style.Width() or MaxWidth() on delegate styles
- **Whitespace at bottom / items not filling screen:** `tea.WindowSizeMsg` may not be received in tmux/iTerm. Detect terminal size at startup in `newModel()`:
  ```go
  // Check LINES env var first (works in tmux)
  if envLines := os.Getenv("LINES"); envLines != "" {
      height, _ = strconv.Atoi(envLines)
  }
  // Fallback to term.GetSize()
  if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
      width, height = w, h
  }
  ```
- **Items starting from wrong position:** Ensure `delegate.SetSpacing(0)` is called in `newModel()` not just in `WindowSizeMsg` handler
- **Dynamic content not showing (e.g., comments):** If delegate height varies by terminal size, set it in BOTH `newModel()` AND `WindowSizeMsg` handler. Use reasonable thresholds (24+ for 1 extra line, 35+ for 2 extra lines):
  ```go
  // In newModel() - detect terminal size at startup
  width, height := 80, 24
  if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
      width, height = w, h
  }
  delegateHeight := 2
  if height >= 35 { delegateHeight = 4 } else if height >= 24 { delegateHeight = 3 }
  delegate.SetHeight(delegateHeight)
  ```
- **Debugging message flow:** Add logging with `%T` format to see actual message types: `logDebug(fmt.Sprintf("msg type: %T", msg))`
- **Pagination instead of continuous scrolling:** If the list shows only a few items and page-jumps when scrolling past them (instead of continuous scrolling), disable filtering with `l.SetFilteringEnabled(false)`. When filtering is enabled, the list uses page-by-page navigation. Also set `l.SetShowPagination(false)` to hide pagination dots and `l.Paginator.PerPage = listHeight / delegateHeight` for proper item count.
- **Terminal detection order:** For bubbletea apps, try `os.Stdin.Fd()` first, then `os.Stdout.Fd()` as fallback. Stdin is preferred for interactive terminal apps.

**Running TUI as subprocess (exec.Command):**
- TUI binaries run via `exec.Command` may lose color capability detection
- The child process doesn't receive proper TTY information from lipgloss/termenv
- **Fix for colors:** Add `CLICOLOR_FORCE=1` to the subprocess environment:
  ```go
  c := exec.Command(tuiPath)
  c.Stdin = os.Stdin
  c.Stdout = os.Stdout
  c.Stderr = os.Stderr
  c.Env = append(os.Environ(), "CLICOLOR_FORCE=1")
  ```
- **Fix for rendering artifacts (duplicated items, overlapping text):** When running bubbletea via exec.Command, the terminal handling gets confused because stdin/stdout are forwarded through the parent process. The bubbletea renderer's diff-based update fails, causing old content not to be cleared. **Solution:** Open `/dev/tty` directly for both input AND output:
  ```go
  // In main() of the TUI binary
  tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
  if err != nil {
      // Fall back to normal mode
      p := tea.NewProgram(newModel(), tea.WithAltScreen())
      p.Run()
      return
  }
  defer tty.Close()
  // Use /dev/tty for both input and output
  p := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithInput(tty), tea.WithOutput(tty))
  p.Run()
  ```
- **Why in-process TUIs work but subprocesses don't:** If TUI runs in the same process (like `tui.RunBranches()` in branches TUI), there's no subprocess terminal forwarding issue. The artifacts only appear when running a separate binary via exec.Command.

**Realtime timers in bubbletea:**
- To have a timer that updates every second without API calls, add a separate fast tick:
  ```go
  type uiTickMsg time.Time
  func uiTickCmd() tea.Cmd {
      return tea.Tick(1*time.Second, func(t time.Time) tea.Msg { return uiTickMsg(t) })
  }
  ```
- Start it in `Init()`: `tea.Batch(..., uiTickCmd())`
- Handle it in `Update()`: `case uiTickMsg: return m, uiTickCmd()` (no API calls, just re-render)
- Calculate duration dynamically in `Render()`: `time.Since(item.StartedAt)`
- Store the actual start time (not pre-formatted duration string) for realtime calculation
- Add debug counter to status line (`tick:N`) to verify tick mechanism is working

**Verifying tick mechanism via tmux:**
```bash
tmux new-session -d -s tui-test -x 120 -y 30 "./md branches tui"
sleep 6 && tmux capture-pane -t tui-test -p | grep "tick:"  # First capture
sleep 3 && tmux capture-pane -t tui-test -p | grep "tick:"  # Second capture, should show higher tick count
```

**Confirmation dialogs in bubbletea:**
- Use model state flags (e.g., `confirmingDelete bool`) to track dialog mode
- Store context in model (e.g., `deleteCandidate BranchInfo`) for the action
- In `Update()`, check dialog state BEFORE handling normal keys:
  ```go
  if m.confirmingDelete {
      switch msg.String() {
      case "y", "Y": return m, performActionCmd(m.deleteCandidate)
      case "n", "N", "esc": m.confirmingDelete = false; return m, nil
      }
      return m, nil // Ignore other keys during confirmation
  }
  ```
- In `View()`, change status/help lines when in confirmation mode
- Add `clearEOL` (`\033[K`) to status/help lines to prevent artifacts when switching modes
- Show success/error messages with auto-clear using timestamp: `if time.Since(m.messageAt) < 5*time.Second`

**Key TUI files:**
- `/Users/davidson/workspace/cli-middesk/tui/branches.go` - Branches TUI implementation
- `md linear tui` - Linear TUI (built into md CLI)
- DefaultDelegate height: controls lines per item (2 = title+desc, 3 = title+desc+url)
