Date: 2026-01-31
Title: Plan - Add README Instructions for React App and am bounce native
status: completed
Related Ticket: 1769891039_README_React_App_Instructions.md

## Analysis

The existing README at `ink-experiment/bounce-view-web/README.md` already covers:
- How to start the React web app (setup, npm commands, running both servers)
- Key bindings for the BounceView
- Architecture overview

Missing from the README:
1. Documentation for `am bounce native` command
2. The async mode (`--async`) flag
3. The `am bounce ls --poll` command for checking/updating workflow status

## Implementation Plan

Add a new section to the README called "## Bounce Commands" that documents:

1. **am bounce native** - The core bounce command that:
   - Triggers Ableton's "Bounce Track In Place" via Control+Option+Command+B
   - Expands parent groups, selects the track, sends keystroke
   - Supports multiple UUIDs (sequential bouncing)
   - Tracks each bounce as a BounceWorkflow in the database

2. **Async Mode** (`--async` flag):
   - Returns immediately after triggering
   - Recommended for batch operations
   - Use with `am bounce ls` to check status

3. **am bounce ls --poll**:
   - Checks and updates status of 'triggered' workflows
   - Used to poll for completion when using async mode

## Files to Modify

- `ink-experiment/bounce-view-web/README.md` - Add bounce command documentation section

### Commands Run / Actions Taken
1. Ran `am` and `am bounce -h` to understand CLI structure
2. Ran `am bounce native -h` to get detailed documentation on the native bounce command
3. Ran `am bounce ls -h` to understand the list/poll commands
4. Read existing README at `ink-experiment/bounce-view-web/README.md`
5. Added new "Bounce Commands" section with documentation for:
   - `am bounce native` command and its behavior
   - `--async` flag for asynchronous operation
   - `am bounce ls` and `--poll` flag for checking status
   - Typical workflow example
6. Ran `make build` - success
7. Ran `make test` - all tests pass

### Results
Added comprehensive documentation to `ink-experiment/bounce-view-web/README.md`:
- Section: "## Bounce Commands"
- Subsections: "am bounce native", "Async Mode", "Checking Bounce Status", "Typical Workflow"
- Includes code examples for all commands
- Documents the workflow from triggering bounces to reconciliation

### Verification Commands / Steps
1. Read the updated README - Verified content is correct and complete
2. `make build` - Build succeeds
3. `make test` - All tests pass
4. Visual inspection of markdown formatting

Verification: 100% complete
- Documentation-only ticket, no runtime behavior to test
- README is properly formatted markdown
- All documented commands match actual CLI help output
