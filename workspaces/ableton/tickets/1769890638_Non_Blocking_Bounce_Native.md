Date: 2026-01-31
Title: Non-Blocking Bounce Native - Return Immediately with Status Command
STATUS: COMPLETED
Dependencies: None
Description: Can we have am bounce native return immediately, and display a help message for the CLI command to view the status of the runs, vs. having it hang?

Currently `am bounce native` blocks while waiting for each bounce to complete (up to 5 minutes per track). This is problematic for:
- CLI scripting and automation
- User experience when bouncing many tracks
- Integration with UIs that want to show progress

The command should:
1. Trigger the bounce(s)
2. Return immediately with the batch_id
3. Show a help message for how to check status (`am bounce ls --batch <batch_id>`)

Comments:

2026-01-31: Implemented `--async` flag for `am bounce native` and `--poll` flag for `am bounce ls`. See plan file for details.

status: completed

