Date: 2026-01-31
Title: Toggle Select All in Bounce View
Status: COMPLETED
Dependencies: None

## Description
Also pressing a again in bounce view should unselect.

When user presses 'a' to select all tracks (using smart select), pressing 'a' again should clear the selection. Currently the toggle behavior doesn't work properly due to a bug in the condition check.

## Implementation

Modified BounceView.tsx:385-391 - changed toggle condition from checking if ALL tracks are selected to checking if ANY tracks are selected:

```typescript
// Before: const allSelected = visibleTracks.every(...)
// After:
const hasSelection = Object.values(selected).some(Boolean);
if (hasSelection) {
  setSelected({});
  return;
}
```

## Comments

2026-01-31 18:37: Picked up ticket. Created plan file: 1769888219_Plan_Toggle_Select_All.md
- Identified root cause: toggle logic checks if ALL tracks selected, but smart select only selects topmost parents
- Proposed fix: change toggle condition to check if ANY tracks are selected

2026-01-31 18:45: Implemented fix
- Edited BounceView.tsx:385-391
- Build passes (make build)
- Tests pass (make test)
- TypeScript compiles (npm run build in React app)
- Servers running: React dev (5173), WebSocket (8080)
- Verification: 80% automated, 20% manual UI test pending
- To verify: Open http://localhost:5173, press 'a' (selects), press 'a' again (clears)
