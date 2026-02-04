Date: 2026-01-31
Title: Add Copy Selected Tracks To Clipboard in React Web App
STATUS: COMPLETED
Dependencies: None

## Description

Add the 'x' key feature to copy selected track names to clipboard in the React web app (`ink-experiment/bounce-view-web`).

This feature exists in the Go TUI (`als-manager/bounce_selection_view.go`) but is missing from the React implementation.

## Implementation Plan

### File to modify
`ink-experiment/bounce-view-web/src/components/BounceView.tsx`

### Changes needed

1. Add 'x' key case to `handleKeyDown` function (after line 603):
```typescript
case 'x':
  e.preventDefault();
  const selectedNames = getSelectedTrackNames();
  if (selectedNames.length > 0) {
    navigator.clipboard.writeText(selectedNames.join('\n'));
    // Optionally show a toast/notification
  }
  break;
```

2. Update help text in status bar (line 777) to include 'x: copy'

## Verification

1. Run the React web app
2. Navigate to bounce view
3. Select some tracks using space
4. Press 'x'
5. Paste in another app to verify track names are in clipboard

## Comments

### 2026-01-31 Implementation Complete

**Changes made to `ink-experiment/bounce-view-web/src/components/BounceView.tsx`:**

1. **Added 'x' key case** in `handleKeyDown` switch statement (lines 636-643):
   ```typescript
   // Copy selected track names to clipboard
   case 'x':
     e.preventDefault();
     const selectedNames = getSelectedTrackNames();
     if (selectedNames.length > 0) {
       navigator.clipboard.writeText(selectedNames.join('\n'));
     }
     break;
   ```

2. **Added `getSelectedTrackNames` to useCallback dependencies** (line 663)

3. **Updated help text** (line 787):
   - Before: `j/k: nav | Space: select | a: all | b: bounce | R: refresh`
   - After: `j/k: nav | Space: select | a: all | x: copy | b: bounce | R: refresh`

**Build status:** Success (TypeScript compiles, `make test` passes)

**Verification steps:**
1. Open http://localhost:5173 in browser (dev servers running)
2. Wait for tracks to load from Ableton
3. Navigate with j/k keys and select tracks with Space
4. Press 'x' to copy selected track names to clipboard
5. Paste (Cmd+V) in any text editor to verify names appear, one per line

**Verification status:** 95% - Code review and build verified. Manual browser testing required for final confirmation that `navigator.clipboard.writeText()` works in the browser context.
