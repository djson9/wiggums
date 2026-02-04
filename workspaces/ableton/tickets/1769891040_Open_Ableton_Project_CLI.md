Date: 2026-01-31
Title: CLI Command to Open Ableton Project by Path
status: completed
Dependencies: None
Description: Please explore ways that we can open '/Users/davidson/Documents/150-6 found the vocal Project/150-6 found the vocal_v1,1.als'. Please add a command under am CLI to open an ableton project by path. We should verify by terminating ableton, trying to open this one, closing any error dialogs, and use OSC to get tracks. We should see many tracks.

Comments:
2026-01-31: Implemented `am project open <path>` command.
- Created cmd/project_open.go with cobra subcommand
- Created handlers/project_open.go with handler logic
- Uses macOS `open` command to launch project in Ableton
- Validates file exists and is .als format
- Optional --wait flag to poll for project load confirmation
- Optional --timeout flag (default 30s) for wait duration
- Tested with specified project: opened successfully with 110 tracks
- Used `am dialog close` to dismiss save dialog when switching projects
- Full end-to-end verification complete

2026-01-31: Documentation added to README.md
- Added to Quick Reference section
- Added full documentation section with usage, flags, examples, output format
- Verified documentation matches actual implementation
- Build passes
- See plan: 1769892357_Document_Project_Open_Command_Plan.md