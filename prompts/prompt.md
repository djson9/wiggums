# Wiggums

This directory holds tickets that the user would like to work on.

**Wiggums Directory:** `{{WIGGUMS_DIR}}`

Your current working directory is the wiggums directory — it contains tickets, prompts, and `.claude` settings. The project codebase is added via `--add-dir` and is where you should do your actual code work. Use `{{WIGGUMS_DIR}}` paths for ticket and prompt operations.

Your assigned ticket is listed at the end of this prompt. Work on ONLY that ticket.

## Ticket Commands

View your assigned ticket:

```bash
wiggums ticket view <ticket-id>
```

List all tickets in this queue:

```bash
wiggums queue inspect {{QUEUE_ID}}
```

View any other ticket in the queue (to see prior work and context):

```bash
wiggums ticket view <other-ticket-id>
```

Update your ticket with progress:

```bash
wiggums ticket update (to create the new sections)
Then populate these sections.
```

## How to Work in This Repo

Please document key decisions and key findings via `wiggums ticket update` as you work.

### Referencing other tickets

Please use markdown format:
[[tickets/test_ticket.md|Hello world title]]
To reference other tickets.

## Beginning the Ticket

IMPORTANT: BEFORE BEGINNING use the explore subagent (just this once) to find tickets that may be related to this one, and read the relevant issues. Use wiggums queue inspect / wiggums ticket view only. Do not scan file system. Be skeptical of the conclusions given by the explore agent, because it sometimes does not return the full context. Use explore agent mostly to find files, but do not trust it's conclusions and read the files yourself.

Do NOT use subagents or tasks or plan mode! (Except for the initial explore agent to look at relevant tickets.)

### Verification Commands / Steps

After implementing you should always verify manually. You should run a set of mental test cases (don't actually create test cases) that allow you to test manually. Think outside the box in ways to test this. Always verify end to end. If making UI changes, verify the UI. If making CLI changes, run the CLI command.

## What does good verification look like?

Please properly verify, or the user will be upset. Here are examples of good and bad verification behavior.

Bad: Verified only that dialog appears, not actually running the command
Bad: Verified only that help menu appears, not actually running the command
Bad: Verified only that code looks correct, not actually running the command
Good: Inspecting state before
Good: Running the command end to end
Good: Inspecting state after
Good: Verifying that the state changed in the expected manner
Bad: Verified only that the backend works, not that the frontend displays it
Bad: Verified only that the command ran, not that the output changed
Bad: Verified only that am audio status shows playing, not that TUI shows ▶
Bad: Checked state once after action without comparing to state before  
Good: Capture TUI output before action
Good: Perform the action
Good: Capture TUI output after action
Good: Verify specific text changed (e.g., ⏹ became ▶, 00:00 became 00:05)

Describes the commands and steps used to verify the result. Note whether we were able to fully test it end to end. Give a percent for the verification completed and % left.

---

On completion of ticket run `wiggums ticket complete <id>`.
