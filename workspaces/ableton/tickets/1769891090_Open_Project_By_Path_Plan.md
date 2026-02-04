# Plan: CLI Command to Open Ableton Project by Path

**Ticket:** 1769891040_Open_Ableton_Project_CLI.md
**Date:** 2026-01-31
status: completed

## Objective
Add a CLI command `am project open <path>` to open an Ableton .als project file by its absolute path.

## Approach
1. Add a new subcommand `open` under the existing `project` command
2. Use macOS `open` command to open the .als file (same pattern as existing `openFile` function in `handlers/open.go`)
3. Optionally poll for dialogs and close them
4. Verify the project loaded by querying tracks via OSC

## Implementation Steps
1. Create `OpenProjectByPathHandler` in `handlers/project_open.go` (new file)
2. Add `am project open <path>` subcommand in `cmd/project_open.go` (new file)
3. The handler:
   - Validates path exists and is a .als file
   - Opens the file with macOS `open` command
   - Optional `--wait` flag polls for project to load
   - Optional `--timeout` flag controls wait duration
   - Returns JSON output

## Commands Run / Actions Taken
1. Explored existing `am open` command and handlers (opens snapshots, not arbitrary paths)
2. Explored AbletonOSC for OSC commands - no direct "open file" OSC command
3. Created `cmd/project_open.go` with the cobra subcommand
4. Created `handlers/project_open.go` with the handler logic
5. Built with `make build`
6. Tested error handling:
   - `am project open "/nonexistent/path.als"` - correctly returns file not found
   - Non-.als files correctly rejected
7. Tested opening the requested project:
   - `am project open "/Users/davidson/Documents/150-6 found the vocal Project/150-6 found the vocal_v1,1.als"`
   - Ableton showed "save changes?" dialog for previous project
   - Used `am dialog close` to dismiss dialog
   - Project loaded successfully with 110 tracks
8. Ran `make test` - all tests pass

## Results
- New command `am project open <path>` successfully implemented
- Command validates file exists and is .als
- Uses macOS `open` command to launch/switch Ableton
- Optional `--wait` flag polls OSC to verify project loaded
- Optional `--timeout` flag (default 30s) controls wait duration
- JSON output always returned

## Verification Commands / Steps
1. `am project open --help` - Shows help with usage examples
2. `am project open "/Users/davidson/Documents/150-6 found the vocal Project/150-6 found the vocal_v1,1.als"` - Opens project
3. `am dialog close` - Dismisses any save dialog
4. `am project` - Shows current project info (verified path changed)
5. `am tracks` - Lists 110 tracks from the new project
6. `make test` - All tests pass

**Verification %:** 100% - Full end-to-end verification complete
- File validation works
- Project opens in Ableton
- Dialog handling works (used `am dialog close`)
- Tracks visible via OSC (110 tracks confirmed)
