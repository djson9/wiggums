# Wiggums

Go CLI that runs Claude Code in a loop to work through a backlog of tickets, organized by workspace.

## How It Works

1. Each workspace has a `tickets/` directory and an `index.md` pointing to an external working directory
2. The CLI finds incomplete tickets, assembles prompts (`prompts/prompt.md` + optional agent prompt), and pipes them to Claude Code
3. After Claude marks a ticket `status: completed`, a verification pass runs (`prompts/verify.md`)
4. Tickets with `MinIterations` are forced through multiple passes before completion is accepted

## CLI Usage

```bash
wiggums ls                              # List available workspaces
wiggums <workspace>                     # Run ticket loop for a workspace
wiggums <workspace> --agent <name>      # Run only tickets matching agent
wiggums workspace <workspace>           # Same as above (alias: w)
```

`--yolo` (default: true) passes `--model opus --dangerously-skip-permissions` to Claude.
`--agent <name>` filters tickets by their `Agent:` frontmatter field and appends `agents/<name>.md` prompt.

## Workspace Structure

```
workspaces/
  <name>/
    index.md          # Frontmatter with Directory: /path/to/working/dir
    shortcuts.md      # Iteration learnings
    tickets/
      [EPOCH]_[Title].md
```

## Ticket Format

Files in `workspaces/<name>/tickets/` with naming: `[EPOCH]_[Title].md`

```markdown
---
Status: not completed
Agent: optional-agent-name
MinIterations: 3
CurIteration: 0
SkipVerification: true
UpdatedAt:
---
# Title

Description of what needs to be done
```

## Key Points

- Mark tickets `Status: completed` when done
- Add `completed + verified` after verification passes
- Tickets with `Agent:` set are only picked up when using `--agent <name>`
- `MinIterations` forces N passes before accepting completion (status resets to `in_progress`)
- `SkipVerification: true` auto-marks tickets as `completed + verified` without running the verifier
- `UpdatedAt` is auto-set by the CLI to the local timestamp after each Claude turn
- Desktop notification via beeep on successful verification
- Claude runs with `--add-dir` pointing back to wiggums so it can read tickets/prompts

## Hook Helpers

`wiggums hook-helpers` provides subcommands for Claude Code hooks to call. The hooks live in `.claude/hooks/` and delegate heavy logic (like JSONL transcript parsing) to Go.

### `should-stop`

Called by the `Stop` hook (`.claude/hooks/conditional-stop.sh`) to decide whether to kill Claude when it finishes responding.

**Logic:**
- 1 human message → kill (normal wiggums loop, single-turn)
- &gt;1 human messages, last is `/wiggums-continue` → kill (user is done interacting)
- &gt;1 human messages, last is anything else → don't kill (user is still chatting)

Tool results in the transcript are filtered out (they appear as `"type":"user"` but have array content vs string content for real human messages).

**Exit codes:** 10 = kill, 0 = don't kill. The bash hook checks this and runs `kill -9 $PPID`.

**Interactive override:** Type `/wiggums-continue` (a Claude Code command in `.claude/commands/`) to signal you're done chatting and the loop should resume.

```bash
wiggums hook-helpers should-stop --dry-run  # test with stdin JSON without killing
```

## Worker Reconciliation (Queue/Ticket Status Desync Fix)

The queue JSON and ticket frontmatter are two independent stores. If the worker dies after Claude completes a ticket but before the queue JSON is updated, the queue still shows `"working"` while the ticket file says `completed`. The `workerReconcileTicket()` function in `worker.go` handles this: before running Claude, it checks the ticket's frontmatter. If already `completed + verified`, it auto-advances. If `completed` (not verified), it runs verification only. This prevents re-running Claude on already-done tickets.

## Architecture: Frontmatter Manipulation

All frontmatter reads/writes live in `cmd/run.go` as standalone functions that scan lines between `---` delimiters. Pattern for adding a new frontmatter field:

1. **Reader**: `extractFrontmatter<Type>(content, key)` — scans frontmatter lines, matches key case-insensitively, returns parsed value
2. **Writer**: `update<FieldName>(path)` — reads file, scans for key, replaces value in-place (or inserts before closing `---` if missing), writes back
3. **Loop wiring**: Call the writer at the appropriate point in `runLoop()` (after `cfg.runner.Run()` for per-turn updates)
4. **Template**: Add the field to `templates/Ticket_Template.md`
5. **Tests**: Add unit test for the function + integration test in `TestRunLoop_*` using `mockRunner`

<IMPORTANT>
The way the queue works is oldest tickets are at the very bottom of the list.
We allow the user to organize uncompleted ticketes, but completed tickets are immutable.
Since completed are at the bottom of the list. The queue works it's way up.
User can freely rearrange tickets that are not worked on, however when queue is active and starts working on a ticket,
the position is immutable for that ticket (until queue is stopped.).
Unworked tickets remain at the top of the queue.
</IMPORTANT>
