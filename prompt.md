# Wiggums

This directory holds bugs and tickets that the user would like to work on.
Please run `pwd` to understand what directory we are in. Please list directories and note the bugs and tickets directories. Also note shortcuts.md.
The user is primarily working out of the parent directory, and this wiggums directory is just a repository for working on bugs and tickets.

## How to Work in This Repo
Pick up the next bug, or if there are no incomplete bugs, look at the next ticket.
Select JUST one ticket to work / bug to work on. Please pick the ticket that will help most with the remaining work. Do not complete more than one uncompleted ticket. Do not pick up extra uncomplete tickets. Do not remove tickets.

Please include execution plan WITHIN the ticket.

Sometimes a ticket is completed, but the grep needs to return nothing to actually complete. So we should add status: completed on any tickets or bugs that are actually completed.

If a human adds a ticket file to the tickets or bugs directory (it likely just be a title and text inside), please add the metadata WITHOUT changing the original content, and have the title match our format as well.

Please read shortcuts.md for tips on iteration shortcuts to help us iterate faster.

### Referencing other tickets
Please use markdown format:
[[tickets/test_ticket.md|Hello world title]]
To reference other tickets or bugs.

## Beginning the Bug / Ticket

IMPORTANT: BEFORE BEGINNING use the explore subagent (just this once) to find  tickets and bugs that may be related to this one, and read the relevant bug and issues.

Add your plan to tackle the bug or ticket in the same file as the ticket / bug.

We should not assume any behaviors about ableton, we should inspect ableton ourselves via the am cli and query the osc to figure out what functionality actually exists.

Do NOT use subagents or tasks! (Except for the initial explore agent to look at relevant tickets and bugs.)

We should at least have these sections, for both bugs and tickets:
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
Finally,
Then mark this ticket as `status: completed  by writing it in the file.

## Filing Bugs
If we encountered ANY bugs (in the CLI, in our tooling, or adjacent to the feature we are building), please file in bugs directory. We should file all bugs in the bugs in the bugs directory before completing.

A bug report should contain reproduction steps, exact commands. It should list potential files to explore. If relevant, theories on what is going on. It should explain user impact.

If the bug prevented the ticket from being fully validated, do NOT set the ticket status to completed. Simply end the turn and return control to the user. We will investigate the bug first and return to the issue.
## Shortcuts
<shortcuts.md>
In each session, there will be workflows unique to this directory that will take some time to figure out. For example, we may stumble around trying to figure out the best way to manually test something. 

Reflect on what workflows took a long term to figure out, specifically thinking about workflows that would be useful for future runs. Try to generalize one layer up, while still citing commands and relevant code. Shortcuts contains learnings that will help us "shortcut" through this hard to understand bits, and iterate faster.

Think deeply about "why was iteration difficult" what breakthroughs helped us to iterate faster? What specific engineering concepts and workflows specific to this repository helped us iterate faster?

Include those learnings and shortcuts in Shortcuts.md, please be as concise as possible.
</shortcuts.md>


