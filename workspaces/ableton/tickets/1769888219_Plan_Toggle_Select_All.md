Date: 2026-01-31
Title: Plan - Toggle Select All in Bounce View
Status: COMPLETED
Dependencies: Depends on 1769886839_Smart_Select_All_Bounce_View.md (COMPLETED)

## Description
When user presses 'a' in bounce view to select all, pressing 'a' again should unselect/clear selection.

## Problem Analysis

Current `selectAll` function (BounceView.tsx:385-433) has toggle logic:
```javascript
const allSelected = visibleTracks.every((idx) => {
  const track = tracks[idx];
  return !track?.is_selectable || hasNoBounceTag(track.name) || selected[String(idx)];
});

if (allSelected) {
  // Toggle off - clear selection
  setSelected(newSelected);
  return;
}
```

The issue: This checks if ALL visible selectable tracks are selected. But smart select only selects topmost bounceable parents, not all children. So the condition `allSelected` is never true after smart select runs.

Example:
1. User presses 'a' - smart select picks 18 topmost bounceable parents
2. User presses 'a' again - `allSelected` checks ALL tracks, but children are NOT selected
3. Toggle never triggers because condition is false

## Proposed Solution

Change the toggle logic to:
1. Check if any tracks are currently selected
2. If yes, clear selection
3. If no, run smart select

This is simpler and more intuitive - pressing 'a' toggles between "smart selection" and "nothing selected".

## Implementation Plan

1. Modify `selectAll` function in BounceView.tsx
2. Replace complex `allSelected` check with simple `Object.values(selected).some(Boolean)`
3. If any tracks selected -> clear selection
4. If no tracks selected -> run smart select algorithm

## Code Change

```typescript
const selectAll = useCallback(() => {
  // Toggle: if any tracks are selected, clear selection
  const hasSelection = Object.values(selected).some(Boolean);
  if (hasSelection) {
    setSelected({});
    return;
  }

  // No selection - run smart select algorithm
  const newSelected: Record<string, boolean> = {};
  const bounceableMemo = new Map<number, boolean>();
  // ... rest of smart select logic
}, [visibleTracks, tracks, selected, hasNoBounceTag, isFullyBounceable]);
```

### Commands Run / Actions Taken
1. Edited BounceView.tsx:385-391 - changed toggle condition
2. Ran `make build` - passed
3. Ran `make test` - passed
4. Ran `npm run build` in React app - passed (no TypeScript errors)

### Results
- Changed toggle logic from checking if ALL tracks selected to checking if ANY tracks selected
- Build passes, tests pass, TypeScript compiles successfully

### Verification Commands / Steps
1. Start React app: `cd ink-experiment/bounce-view-web && npm run dev:all`
2. Open http://localhost:5173
3. Press 'a' - verify smart select works (status shows N selected)
4. Press 'a' again - verify selection clears (status shows 0 selected)
5. Press 'a' again - verify smart select works again
6. Select tracks manually with space, then press 'a' - verify it clears

**Verification Status: 80%**
- Code change verified syntactically correct
- Build and tests pass
- Remaining 20%: Manual UI testing in browser
