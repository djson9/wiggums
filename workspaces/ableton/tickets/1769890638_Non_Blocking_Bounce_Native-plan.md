Date: 2026-01-31
Title: Plan - Non-Blocking Bounce Native
status: completed
Related Ticket: 1769890638_Non_Blocking_Bounce_Native.md

## Analysis

The current `BounceNativeHandler` in `ableton-manager-cli/handlers/bounce.go` (lines 1496-1938):

1. Validates UUIDs and creates workflow records (status: "queued")
2. For each track, sequentially:
   - Updates status to "in_progress"
   - Selects track and sends bounce keystroke
   - Updates status to "triggered"
   - **BLOCKS**: Polls every 500ms waiting for completion (up to 5 min timeout)
   - Updates status to "completed" or "failed"
3. Returns JSON with all results

The blocking happens in the polling loop (lines 1768-1905).

## Approach

Add a `--async` flag to `am bounce native` that:
1. Triggers all bounces but doesn't wait for completion
2. Returns immediately with batch_id and queued workflow IDs
3. Shows help message for checking status

When `--async` is used:
- Still create workflow records (queued → triggered)
- Return immediately after triggering each bounce
- User checks status via `am bounce ls --batch <batch_id>`

The existing workflow tracking infrastructure handles status updates via completion detection.

**Issue**: Completion detection currently runs in the same process. With async mode, we need a way to detect completion asynchronously.

## Options

### Option A: Async flag with background worker (Complex)
- Add `--async` flag
- Spawn goroutine or separate process to monitor completion
- Pro: Full async behavior
- Con: Complex, needs process management

### Option B: Async flag with manual status check (Simple)
- Add `--async` flag
- When async, trigger bounces but return immediately with status "triggered"
- User manually runs `am bounce native --continue` or just checks status
- Completion detection happens on next command that queries tracks
- Pro: Simple, no background processes
- Con: Status stays "triggered" until explicitly checked

### Option C: Fire-and-forget with polling command (Simpler)
- Add `--async` flag to trigger and return immediately
- Add new command `am bounce poll` that checks triggered workflows and updates their status
- User runs poll periodically to update statuses
- Pro: Very simple, stateless
- Con: Requires manual polling

**Decision**: Option B with enhancement - add a `--poll` flag to `am bounce ls` that also checks and updates triggered workflow statuses.

## Implementation Plan

1. Add `--async` flag to `bounceNativeCmd`
2. Modify `BounceNativeHandler` to:
   - When async: trigger bounce, set status to "triggered", don't wait, continue to next track
   - Return immediately with batch_id and help message
3. Modify `BounceListHandler` to accept `--poll` flag that:
   - For each "triggered" workflow, check if bounce completed
   - Update status accordingly
4. Test with React app and CLI

## Files to Modify

- `ableton-manager-cli/cmd/bounce.go` - Add flags
- `ableton-manager-cli/handlers/bounce.go` - Modify handlers

## Verification

1. Run `am bounce native <uuid> --async` - should return immediately
2. Run `am bounce ls --batch <batch_id>` - should show triggered status
3. Wait for bounce to complete in Ableton
4. Run `am bounce ls --poll --batch <batch_id>` - should detect completion and update status
5. Test via React app bounce flow

---

## Implementation (Completed 2026-01-31)

### Commands Run / Actions Taken

1. Added `--async` flag to `bounceNativeCmd` in `ableton-manager-cli/cmd/bounce.go`
2. Added `--poll` flag to `bounceListCmd` in `ableton-manager-cli/cmd/bounce.go`
3. Modified `BounceNativeHandler` in `ableton-manager-cli/handlers/bounce.go`:
   - Added async mode check after triggering bounce
   - In async mode: record result as "triggered" and continue to next track without waiting
   - Return early with batch_id and help message showing status/poll commands
4. Modified `BounceListHandler` in `ableton-manager-cli/handlers/bounce.go`:
   - Added poll mode support
   - Added "triggered" status to batch_summary
   - Created `pollTriggeredWorkflows` helper function to check and update triggered workflow statuses
5. Built project: `make build`
6. Tested with CLI

### Results

- `am bounce native --async` returns immediately after triggering bounces (~2 seconds)
- Returns JSON with `"async": true`, `"triggered": N`, and helpful message
- `am bounce ls --batch <batch_id>` shows workflows with "triggered" status
- `am bounce ls --poll` checks triggered workflows and updates their status:
  - For MIDI/Group tracks: detects type change to audio and marks completed
  - For audio tracks: reports that manual verification needed (no original clip info stored)
- All tests pass

### Verification Commands / Steps

1. `am bounce native fe3c8 --async` - Returns immediately with batch_id ✓
2. `am bounce ls --batch <batch_id>` - Shows status "triggered" ✓
3. `am bounce ls --poll --batch <batch_id>` - Updates MIDI track status to "completed" when detected ✓
4. `make test` - All tests pass ✓

**Verification completed: 90%**

Remaining 10%:
- Audio track poll detection is limited (cannot determine completion without original clip info)
- React app integration not tested (uses different bounce method)