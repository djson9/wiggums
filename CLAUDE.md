# Wiggums

Go CLI that runs Claude Code in a loop to work through a backlog of tickets, organized by workspace.

## How It Works

1. Each workspace has a `tickets/` directory and an `index.md` pointing to an external working directory
2. The CLI finds incomplete tickets, assembles prompts (`prompts/prompt.md` + optional agent prompt), and pipes them to Claude Code
3. After Claude marks a ticket `status: completed`, a verification pass runs (`prompts/verify.md`)
4. Tickets with `MinIterations` are forced through multiple passes before completion is accepted

## CLI Usage

```bash
wiggums ls                      # List available workspaces
wiggums <workspace>             # Run ticket loop for a workspace
wiggums workspace <workspace>   # Same as above (alias: w)
wiggums run                     # Run against root tickets/ dir
wiggums run agent <name>        # Filter tickets by Agent field
```

`--yolo` (default: true) passes `--model opus --dangerously-skip-permissions` to Claude.

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
---
# Title

Description of what needs to be done
```

## Key Points

- Mark tickets `Status: completed` when done
- Add `completed + verified` after verification passes
- Tickets with `Agent:` set are only picked up by `wiggums run agent <name>`
- `MinIterations` forces N passes before accepting completion (status resets to `in_progress`)
- Desktop notification via beeep on successful verification
- Claude runs with `--add-dir` pointing back to wiggums so it can read tickets/prompts
