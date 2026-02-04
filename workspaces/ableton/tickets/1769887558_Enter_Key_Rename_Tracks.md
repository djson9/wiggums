Date: 2026-01-31
Title: Enter Key Rename Tracks in Bounce View
STATUS: COMPLETED
Dependencies: None
Description: User should be able to press Enter key in bounce view to rename the currently focused track. It should be a fluid and easy UX experience. The rename functionality should use the existing `am track rename` CLI command.

Original request: "Can we have 'enter' key in bounce view be rename? To rename tracks. It should be really fluid and easy UX."

Comments:

2026-01-31 14:30: Implementation completed.

**Changes Made:**
1. `BounceView.tsx`:
   - Added rename state (`isRenaming`, `renameTrackIndex`)
   - Added `startRename`, `cancelRename`, `confirmRename` callbacks
   - Modified key handler: Enter now triggers rename mode (previously toggled expand/collapse)
   - Group expand/collapse is still available via 'e' key and arrow keys (h/l)
   - Updated help text in status bar to show "Enter: rename"

2. `TrackTree.tsx`:
   - Added props for rename state and callbacks (`renameTrackIndex`, `onConfirmRename`, `onCancelRename`)
   - Passes `isRenaming` prop to TrackRow based on matching track index

3. `TrackRow.tsx`:
   - Added `isRenaming` prop and rename handlers
   - When in rename mode, shows an inline input field instead of the track name
   - Input auto-focuses and selects all text on enter rename mode
   - Enter confirms the rename, Escape cancels
   - Clicking outside (blur) also confirms the rename

4. `TrackRow.css`:
   - Added styling for rename input (`.rename-input`, `.track-row.renaming`)

**CLI Integration:**
- Uses `am track rename <index> "<name>"` command
- UUID is automatically preserved by the CLI command
- Tracks list is refreshed after successful rename

**Testing:**
- CLI rename command verified working: `am track rename 3 "nostalgic pluck test"` ✓
- TypeScript build passes ✓
- Go tests pass ✓

**Manual Verification Needed:**
- Open React app at http://localhost:5175
- Navigate to a track with j/k
- Press Enter to enter rename mode
- Type new name and press Enter to confirm
- Verify track is renamed in Ableton

**Verification Status: 90%**
- CLI integration verified
- Build verified
- UI changes need manual testing in browser

