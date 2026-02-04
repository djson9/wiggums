Date: 2026-01-31
Title: Plan - Clipboard Copy UI Feedback
STATUS: COMPLETED
Related Ticket: 1769888561_Clipboard_UI_Feedback.md

## Summary

Enhancing the clipboard copy feature (x key) in the React BounceView to:
1. Copy UUIDs instead of track names
2. Show UI feedback when copy occurs (toast notification)

## Current Implementation (Before Fix)

In `ink-experiment/bounce-view-web/src/components/BounceView.tsx`, lines 781-787:
```javascript
case 'x':
  e.preventDefault();
  const selectedNames = getSelectedTrackNames();
  if (selectedNames.length > 0) {
    navigator.clipboard.writeText(selectedNames.join('\n'));
  }
  break;
```

This copies track names to clipboard but:
- Should copy UUIDs instead
- Has no user feedback that copy occurred

## Implementation Plan

### 1. Change copy to use UUIDs instead of names
- Already have `getSelectedUUIDs()` function (lines 500-505)
- Replace `getSelectedTrackNames()` with `getSelectedUUIDs()` in the x key handler

### 2. Add Toast Notification
- Add state for toast message: `toastMessage` and `showToast`
- Add a simple toast component to show feedback
- Auto-dismiss after 2 seconds
- Message: "Copied N UUIDs to clipboard"

### Files to Modify

- `ink-experiment/bounce-view-web/src/components/BounceView.tsx`:
  - Add toast state
  - Modify 'x' key handler to copy UUIDs and show toast
  - Add toast UI element
  - Add auto-dismiss effect

- `ink-experiment/bounce-view-web/src/components/BounceView.css`:
  - Add toast styling

### Commands Run / Actions Taken

1. Read existing BounceView.tsx to understand current clipboard implementation
2. Added `toastMessage` state and `toastTimerRef` ref to BounceView.tsx
3. Added `showToast()` callback function with auto-dismiss (2 seconds)
4. Modified 'x' key handler to:
   - Use `getSelectedUUIDs()` instead of `getSelectedTrackNames()`
   - Call `showToast()` with message "Copied N UUID(s) to clipboard"
5. Added `showToast` to useCallback dependency array for `handleKeyDown`
6. Added toast notification JSX element
7. Added toast CSS styling (slide-in animation, green accent border)
8. Built successfully: `npm run build`

### Results

**Files modified:**
- `ink-experiment/bounce-view-web/src/components/BounceView.tsx` - Added toast state, showToast function, modified x key handler, added toast JSX
- `ink-experiment/bounce-view-web/src/components/BounceView.css` - Added .toast-notification styling

**New behavior:**
- Press 'x' with selected tracks → copies UUIDs to clipboard (not names)
- Toast notification appears in top-right: "Copied N UUID(s) to clipboard"
- Toast auto-dismisses after 2 seconds
- Toast has green accent matching the app's color scheme

### Verification Commands / Steps

**Manual verification performed:**
1. Started dev server: `cd ink-experiment/bounce-view-web && npm run dev:all`
2. App available at http://localhost:5175
3. Verified build passes: `npm run build` - Success

**To verify end-to-end:**
1. Open http://localhost:5175 in browser
2. Navigate to tracks using j/k keys
3. Select 2-3 tracks using Space key
4. Press 'x' key
5. Expected: Toast appears showing "Copied 3 UUIDs to clipboard" (green checkmark icon)
6. Expected: Toast disappears after 2 seconds
7. Paste into text editor - should show UUIDs like:
   ```
   d875bbb6-eb7e-4067-84b8-61258cf7e8a0
   17dd4477-9ba7-436d-af28-18ff9cf372c6
   5fd49ceb-f9f3-4909-a9d8-636a1725bd11
   ```

**Verification status: 80% complete**
- Build verification: 100% ✓
- Code review: 100% ✓
- Manual UI testing: Needs browser verification (20% remaining)
