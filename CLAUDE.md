# Wiggums

Ticket/task automation system that runs Claude Code in a loop to work through a backlog of tickets.

## How It Works

1. `wiggums.sh` runs an infinite loop finding incomplete tickets
2. Pipes prompts to Claude Code to work on them
3. Verifies completed work
4. Continues until all tickets are done

## Running

```bash
./wiggums.sh
```

## Ticket Format

Files go in `tickets/` with naming: `[EPOCH]_[Title].md`

```markdown
Date: 2025-01-15
Title: Feature Name
Status: [completed | not completed]
Dependencies: Other ticket if needed
Description: What needs to be done

Comments:
2025-01-15 10:30: Did X, result was Y
```

## Key Points

- Mark tickets `status: completed` when done
- Add `completed + verified` after verification passes
- The system has access to parent directory (`../`) for working on other code
- Tickets track work; actual code changes happen in the parent repo
