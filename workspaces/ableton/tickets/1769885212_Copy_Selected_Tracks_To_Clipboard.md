Date: 2026-01-31
Title: Copy Selected Tracks To Clipboard (x key in bounce view)
STATUS: COMPLETED
Dependencies: None

## Description

User should be able to press 'x' in bounce view to copy all selected track names into the Mac clipboard. This allows easy sharing of selected track names externally (e.g., pasting into notes, documentation, or other tools).

## Original Request

I should be able to press x in bounce view, to copy all of the tracks I have selected into my mac clipboard

## Implementation Plan

### Analysis

1. **Bounce view structure**: Located in `/Users/davidson/workspace/ableton-bouncer/als-manager/bounce_selection_view.go`
2. **Key bindings**: Defined in `bounceKeyActions` array (lines 40-108)
3. **Selection tracking**: `v.selected map[int]bool` stores selected track indices
4. **Track data**: `v.tracks []BounceTrack` contains track info including `Name` field

### Implementation Steps

1. **Add key binding for 'x'** (after line 87 in bounceKeyActions):
   ```go
   makeBounceAction(actionKeys("x"), actionHelp("x", "copy names"), (*BounceSelectionViewV2).handleCopySelectedToClipboard),
   ```

2. **Implement handler function** `handleCopySelectedToClipboard`:
   - Iterate through `v.selected` to find selected tracks
   - Collect track names from `v.tracks`
   - Join names with newlines
   - Use `os/exec` to pipe to `pbcopy` (macOS native clipboard)
   - Show brief feedback message (optional: use dialog system)

3. **No external dependencies needed**: Use `os/exec` with `pbcopy` which is native to macOS

### Code Pattern

Following existing patterns from `handleDelete` (line 1308):
```go
func (v *BounceSelectionViewV2) handleCopySelectedToClipboard() tea.Cmd {
    // 1. Collect selected track names
    var names []string
    for i, selected := range v.selected {
        if selected {
            for _, track := range v.tracks {
                if track.Index == i {
                    names = append(names, track.Name)
                    break
                }
            }
        }
    }

    // 2. If no selection, do nothing
    if len(names) == 0 {
        return nil
    }

    // 3. Copy to clipboard using pbcopy
    text := strings.Join(names, "\n")
    cmd := exec.Command("pbcopy")
    cmd.Stdin = strings.NewReader(text)
    cmd.Run()

    return nil
}
```

## Verification

1. Open als-manager TUI: `am tmux screen`
2. Navigate to bounce view by pressing 'b'
3. Select some tracks using space
4. Press 'x'
5. Open a text editor (e.g., Notes, VS Code)
6. Paste (Cmd+V) and verify track names appear

## Comments

### 2026-01-31 Implementation Complete

**Changes made:**

1. **File modified:** `/Users/davidson/workspace/ableton-bouncer/als-manager/bounce_selection_view.go`

2. **Added import:** `os/exec` (line 7)

3. **Added key binding** (line 89):
   ```go
   makeBounceAction(actionKeys("x"), actionHelp("x", "copy names"), (*BounceSelectionViewV2).handleCopySelectedToClipboard),
   ```

4. **Added handler function** `handleCopySelectedToClipboard()` (after line 1325):
   - Collects names of all selected tracks
   - If no tracks selected, does nothing
   - Joins names with newlines and pipes to `pbcopy` (macOS native clipboard)
   - Logs the operation for debugging with `--verbose`

**Build status:** Success (all tests pass)

**How to verify:**
1. Start tmux session: `am tmux start` (if not already running)
2. View the screen: `am tmux screen`
3. Press 'b' to enter bounce view
4. Use space to select some tracks (see green checkmarks)
5. Press 'x' to copy selected track names to clipboard
6. Paste anywhere (Cmd+V) to see the track names, one per line

**Note:** The 'x' key shows in help footer as "x: copy names"
