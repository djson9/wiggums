Date: 2026-01-31
Title: Plan - Enter Key Rename Tracks in Bounce View
STATUS: COMPLETED
Related Ticket: 1769887558_Enter_Key_Rename_Tracks.md

## Problem
User wants to press Enter on a track to rename it directly from the bounce view. Currently Enter toggles expand/collapse for groups.

## Investigation

### Current Behavior (BounceView.tsx lines 645-649)
- Enter key currently toggles expand/collapse for group tracks
- For non-group tracks, Enter does nothing useful

### CLI Support
The `am track rename` command exists and supports:
- `am track rename <index> <name>` - Rename track by index
- Preserves UUID by default, use --raw to strip

### UX Design
For a "fluid" rename experience:
1. User presses Enter on a track
2. An inline text input appears over the track name
3. User types the new name
4. Press Enter to confirm, Escape to cancel
5. Track is renamed via CLI, list refreshes

## Implementation Plan

### 1. Add Rename State to BounceView.tsx
- `isRenaming: boolean` - Whether we're in rename mode
- `renameTrackIndex: number | null` - Which track is being renamed
- `renameValue: string` - Current input value

### 2. Modify Key Handler
- Enter key:
  - If on a group and NOT already in rename mode: toggle expand (preserve current behavior)
  - If on any track and pressing Enter while NOT in rename mode: enter rename mode
  - Decision: Use 'r' for rename, keep Enter for expand/collapse (less disruptive UX change)

Actually, reconsidering: The user explicitly asked for Enter to be rename. So we'll:
- Enter: Start rename mode for current track
- For groups that need expand/collapse: keep 'e' or arrow keys (which already work)

### 3. Create RenameInput Component or Inline Input
- Show in TrackTree when track is in rename mode
- Auto-focus the input
- Pre-populate with current track name
- Handle Enter to confirm, Escape to cancel

### 4. Implement Rename Handler
```typescript
const handleRename = async (trackIndex: number, newName: string) => {
  await sendCommandWithResponse(`track rename ${trackIndex} "${newName}"`);
  await loadTracks(); // Refresh
};
```

### 5. Update Status Bar
- Show "Enter: rename" in help text instead of current behavior

## Files to Modify
1. `ink-experiment/bounce-view-web/src/components/BounceView.tsx` - Main logic
2. `ink-experiment/bounce-view-web/src/components/TrackTree.tsx` - Inline input rendering
3. `ink-experiment/bounce-view-web/src/components/BounceView.css` - Styling for rename input

## Test Cases (Manual)
1. Press Enter on a non-group track - Should show rename input
2. Type new name and press Enter - Should rename and refresh
3. Press Escape during rename - Should cancel without changes
4. Press Enter on a group track - Should show rename input (group can be renamed too)
5. Rename preserves UUID - Verify track UUID is preserved after rename
