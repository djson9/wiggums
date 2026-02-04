---
Date: 2026-02-01
Title: Ensure Bounce View Uses New Bounce Flow
Status: completed
Dependencies: [[tickets/1769987677_Audio_Daemon_FFPlay_Backend.md|Audio Daemon ffplay Backend]]
---

## Description
We should ensure bounce view's bounce uses the new bounce flow (react app does it correctly, reference that).

## Context
The React app has the correct bounce flow implementation. The TUI bounce view should follow the same pattern to ensure consistency.

## Investigation Completed

### React App Bounce Flow (correct)
Location: `ink-experiment/bounce-view-web/src/components/BounceView.tsx`

1. User presses 'b' → calls `prepareBounce(uuids)` to show confirmation dialog
2. Confirmation dialog shows bounce preview info from `am bounce preview <uuids>`
3. User confirms → calls `confirmBounce()`:
   - Calls `am bounce batch <uuids>` which returns immediately with `batch_id`
   - Polls `am bounce ls --batch <batchId> --poll` every 2 seconds
   - Updates progress display (completed count, current track name)
   - When all tracks complete, refreshes tracks and closes dialog

### TUI Bounce Flow (current - incorrect)
Location: `als-manager/controller_bounce_flow.go` and `als-manager/script_run_bounce.go`

1. User presses 'y' on confirmation → `handleBounceConfirmYes()` → `startBounceFlowDirect()`
2. Creates a `ScriptTask` with:
   - `CLICommand = "bounce"`
   - `CLIArgs = uuids`
3. `executeCLIScript()` runs `am bounce --verbose <uuids>`

**Problem:** `am bounce <uuids>` is not a valid command! The bounce command requires a subcommand:
- `am bounce batch <uuids>` - queue for daemon processing (returns immediately)
- `am bounce one <uuids>` - wait for each bounce to complete

### Fix Required
Change the TUI to use `bounce batch <uuids>` instead of `bounce <uuids>`.

The simplest fix is to update `controller_bounce_flow.go:handleBouncePrep()`:
```go
task.CLICommand = "bounce batch"  // Changed from "bounce"
task.CLIArgs = uuids
```

This will:
1. Queue tracks for daemon processing
2. Return immediately with progress messages
3. Let the daemon handle actual bouncing

## Plan

1. Update `handleBouncePrep()` to use `bounce batch` command
2. Build and test with headless TUI
3. Verify bounces are queued and processed correctly

### Commands Run / Actions Taken

1. Updated `als-manager/controller_bounce_flow.go` line 216:
   - Changed `task.CLICommand = "bounce"` to `task.CLICommand = "bounce batch"`

2. Built the project: `make build`

3. Tested bounce batch command directly via CLI:
   ```bash
   am bounce batch fe3c8075-03ef-4b8e-adbc-45e6e2a4d879 --verbose
   ```
   Result: Successfully queued track, daemon auto-started, returned batch_id

4. Monitored bounce progress:
   ```bash
   am bounce ls --batch 25cd637a
   ```
   Result: Status progressed from `in_progress` to `completed`

5. Verified daemon logs showed successful bounce completion:
   ```
   [2026-02-01 18:38:52] Starting batch 25cd637a with 1 tracks
   [2026-02-01 18:38:52] [1/1] Bouncing 28-KSHMR_Open_Hat_12_Natural (fe3c8075)
   [2026-02-01 18:39:55] [1/1] Completed: 28-KSHMR_Open_Hat_12_Natural
   [2026-02-01 18:39:55] Batch 25cd637a completed successfully
   ```

### Results

- Fix implemented: TUI now uses `bounce batch` command instead of invalid `bounce` command
- CLI verification: `am bounce batch` works correctly (queues, returns immediately, daemon processes)
- Bounce daemon: Auto-starts, processes batches, completes successfully
- TUI-to-React parity: TUI now uses same CLI command pattern as React app

### Verification Commands / Steps

**Verified:**
1. ✅ `am bounce batch <uuid>` - queues track for daemon processing
2. ✅ `am bounce ls --batch <batchId>` - shows batch status
3. ✅ Daemon logs show bounce completion
4. ✅ Track bounce completed successfully

**Partial verification (headless TUI navigation issues):**
- Could not fully test TUI bounce flow end-to-end due to cursor navigation not working in headless mode
- The store daemon references in Shortcuts.md appear to be outdated
- CLI-level testing confirms the fix is correct

**Verification: 80% complete**
- CLI path fully verified
- TUI integration needs manual testing (cursor navigation in headless mode blocked)

---

### 2026-02-01 18:48 - FURTHER VERIFICATION NEEDED

**Blocker Found:** TUI bounce flow fails with `ERROR: Cannot send updateScriptPopupMsg - p.program is nil`

**Evidence from development.log:**
```
[2026-02-01 18:48:36.970] [GO] handleBouncePrep: Preparing bounce script
[2026-02-01 18:48:36.970] [GO] Selected tracks for bounce: [3]
[2026-02-01 18:48:37.819] [GO] SEQUENTIAL EXECUTION: Starting script 1 of 4: Bouncing 1 Tracks (Type: bounce)
[2026-02-01 18:48:37.819] [GO] Sending updateScriptPopupMsg, p.program=false
[2026-02-01 18:48:37.819] [GO] ERROR: Cannot send updateScriptPopupMsg - p.program is nil
```

**What's Working:**
- Code change is correct: `handleBouncePrep` sets `task.CLICommand = "bounce batch"` (line 217)
- TUI navigation works: can press 'b', see confirmation dialog, press 'y'
- Pre-hook flow is triggered correctly

**What's NOT Working:**
- ScriptPopup has nil `p.program` reference in headless mode
- Bounce never actually executes because script execution fails early

**To Verify (once bug is fixed):**
1. Start headless TUI
2. Select a track, press 'b', press 'y'
3. Verify `am bounce ls` shows a new batch with `in_progress` or `completed` status
4. Verify bounce daemon log shows the track being processed

**Related Bug Filed:** [[bugs/1769989796_ScriptPopup_Program_Nil_Headless.md|ScriptPopup p.program nil in headless mode]]

---

### 2026-02-01 18:56 - VERIFICATION COMPLETE

**Blocker Resolved:** The bug [[bugs/1769989796_ScriptPopup_Program_Nil_Headless.md|ScriptPopup p.program nil in headless mode]] has been fixed.

**End-to-End Verification:**

1. Started headless TUI: `./als-manager --headless`
2. Verified program reference set: `grep "Program reference set" development.log` → `m.program=true`
3. Navigated to MIDI track (nostalgic pluck): `curl -s "http://127.0.0.1:9877/key?k=j"` (3 times)
4. Selected track: `curl -s "http://127.0.0.1:9877/key?k=space"`
5. Started bounce: `curl -s "http://127.0.0.1:9877/key?k=b"` → confirmation dialog appeared
6. Confirmed bounce: `curl -s "http://127.0.0.1:9877/key?k=y"`
7. Verified batch created: `am bounce ls | head -1` → `c1f89828 [in_progress]`
8. Waited for completion: `am bounce ls --batch c1f89828` → `status: completed`
9. Verified track bounced: `am track inspect <uuid>` → clips now have "(Bounce)" in name, `has_midi_input: null` (converted from MIDI to audio)

**Verification: 100% complete**

All verification steps passed:
- ✅ TUI uses `bounce batch` command (code change verified)
- ✅ Confirmation dialog shows correctly
- ✅ Bounce batch is created upon confirmation
- ✅ Daemon processes the batch
- ✅ Track is successfully bounced (clips renamed, MIDI→audio conversion)
- ✅ No "p.program is nil" errors during bounce flow
