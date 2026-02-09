# Wiggums

This directory holds tickets that the user would like to work on.

**Wiggums Directory:** `{{WIGGUMS_DIR}}`

Your current working directory is the primary codebase you should be working in. The wiggums directory above is added as an additional directory — use it for reading/writing tickets and shortcuts, but do your actual code work in the current working directory.

The remaining incomplete tickets are listed at the end of this prompt. Pick up the next one to work on.

## How to Work in This Repo
Pick up the next ticket. Only work on tickets in the tickets/ folder.
Select JUST one ticket to work on. Please pick the ticket that will help most with the remaining work. Do not complete more than one uncompleted ticket. Do not pick up extra uncomplete tickets. Do not remove tickets.

Please include execution plan WITHIN the ticket.

Sometimes a ticket is completed, but the grep needs to return nothing to actually complete. So we should add status: completed on any tickets that are actually completed.

If a human adds a ticket file to the tickets directory (it likely just be a title and text inside), please add the metadata WITHOUT changing the original content, and have the title match our format as well.

Please read `{{SHORTCUTS_PATH}}` for tips on iteration shortcuts to help us iterate faster.

### Referencing other tickets
Please use markdown format:
[[tickets/test_ticket.md|Hello world title]]
To reference other tickets.

## Beginning the Ticket

IMPORTANT: BEFORE BEGINNING use the explore subagent (just this once) to find tickets that may be related to this one, and read the relevant issues.

To understand the state of the repo, try running `git diff master...`

Add your plan to tackle the ticket in the same file as the ticket.

Do NOT use subagents or tasks! (Except for the initial explore agent to look at relevant tickets.)

We should at least have these sections for tickets:

### Additional Context
If we gathered any additional context at the request of the user, describe it here. This could include additional context gathered from github, linear, slack, etc.
### Commands Run / Actions Taken
Describe commands run and actions take to achieve the result.
### Results
Describe the result and what was accomplished.
### Verification Commands / Steps
After implementing you should always verify manually. You should run a set of mental test cases (don't actually create test cases) that allow you to test manually. Think outside the box in ways to test this. Always verify end to end. If making UI changes, verify the UI. If making CLI changes, run the CLI command.

## What does good verification look like?
Please properly verify, or the user will be upset. Here are examples of good and bad verification behavior.

Bad:  Verified only that dialog appears, not actually running the command
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


----
<IMPORTANT>
When finished with ANY ticket, mark it as `status: completed` by writing it in the header of the file.
</IMPORTANT>

## Shortcuts
<shortcuts.md>
In each session, there will be workflows that will take some time to figure out. For example, we may stumble around trying to figure out the best way to manually test something.

Reflect on what workflows took a long term to figure out, specifically thinking about workflows that would be useful for future runs. Try to generalize one layer up, while still citing commands and relevant code. Shortcuts contains learnings that will help us "shortcut" through this hard to understand bits, and iterate faster.

Think deeply about "why was iteration difficult" what breakthroughs helped us to iterate faster? What specific engineering concepts and workflows helped us iterate faster?

Include those learnings and shortcuts in `{{SHORTCUTS_PATH}}`, please be as concise as possible.
</shortcuts.md>


