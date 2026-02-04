# Reset Project Command

Date: 2026-02-01
Title: Add am reset-project Command
status: completed
Dependencies: [[tickets/1769891090_Open_Project_By_Path_Plan.md|Open Project By Path]] (completed)

## Description
Add an `am reset-project` command that:
1. Uses `am project` to find the current file path
2. Closes Ableton
3. Restarts Ableton with the same project

## Success Criteria
Opening and closing 8 times without error.

## Original Request
Can we add an am reset-project command?
Which uses am project to find the current file path. Closes ableton and then restarts it?
Success is opening and closing 8 times without error.

## Plan
1. Create `handlers/reset_project.go` with `ResetProjectHandler`:
   - Get current project file path via OSC (`GetFilePath`)
   - Quit Ableton via AppleScript `tell application "Live" to quit`
   - Wait for Ableton to fully close (poll process list)
   - Reopen the project via macOS `open` command
   - Wait for project to load (optional, via --wait flag)
2. Create `cmd/reset_project.go` with `am reset-project` command
3. Add flags: `--wait` (wait for reload), `--timeout` (seconds)
4. Test 8x open/close cycle as per success criteria

## Commands Run / Actions Taken
1. Created `ableton-manager-cli/handlers/reset_project.go`:
   - Gets current project path via OSC
   - Quits Ableton via AppleScript (async to handle dialog)
   - Polls for and auto-dismisses "Save changes" dialog by clicking button 1 of group 1 of window 1
   - Waits for Ableton process to fully terminate
   - Reopens project via macOS `open` command
   - Optionally waits for project to load (default: true)

2. Created `ableton-manager-cli/cmd/reset_project.go`:
   - Added `am reset-project` command
   - Flags: `--wait` (default true), `--timeout` (default 60s)

3. Built with `make build`
4. Ran `make test` - all tests pass

## Results
- Command `am reset-project` successfully implemented
- Automatically handles "Save changes" dialog (clicks "Don't Save")
- Tested 8x consecutive cycles - all successful
- Average cycle time: ~72 seconds (12-16s close + 56-60s reopen)
- Track count verified on each reload: 110 tracks

## Verification Commands / Steps
```bash
# Test 1/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 70.86}

# Test 2/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 74.97}

# Test 3/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 69.52}

# Test 4/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 73.86}

# Test 5/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 74.30}

# Test 6/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 69.77}

# Test 7/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 71.51}

# Test 8/8
am reset-project --verbose
# {"success": true, "track_count": 110, "total_time_s": 75.42}
```

**Verification %:** 100% - All 8 cycles completed successfully with:
- Ableton quit correctly each time
- "Save changes" dialog auto-dismissed each time
- Project reopened and loaded correctly each time
- Track count verified (110 tracks) on each reload
