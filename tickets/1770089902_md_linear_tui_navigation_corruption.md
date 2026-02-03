---
Date: 2026-02-02
Title: md linear tui navigation character corruption
Status: completed + verified
Description: When navigating up and down in md linear tui using j/k keys, characters get corrupted/duplicated. Ghost text from previous items appears when scrolling.
---

## Original Request
In md linear tui when I navigate up and down, the characters seem to get corrupted.

```
╭───────────────────╮╭────────────────╮╭───────────────╮╭─────────────╮
│  In Progress (5)  ││  Planned (10)  ││  Backlog (6)  ││  Done (65)  │
╰───────────────────╯╰────────────────╯╰───────────────╯╰─────────────╯


│ JAY-190: Rosanna PR review
╭───────────────────╮╭────────────────╮╭───────────────╮╭─────────────╮
│  In Progress (5)  ││  Planned (10)  ││  Backlog (6)  ││  Done (65)  │
╰───────────────────╯╰────────────────╯╰───────────────╯╰─────────────╯


│ JAY-190: Rosanna PR review
│ https://linear.app/jays-work-desk/issue/JAY-190/rosanna-pr-review

│ JAY-190: Rosanna PR review
│ https://linear.app/jays-work-desk/issue/JAY-190/rosanna-pr-review             JAY-185: Geocoder revert reverted changes in Middesk...
```

---

## Related Tickets/Bugs
- [[tickets/1770084000_md_branches_tui_list_items_not_extending.md|md branches tui list items not extending]] - Same rendering artifact issue was fixed here with ANSI clear-to-end-of-line codes

---

## Root Cause Analysis

The `View()` function in `/Users/davidson/workspace/linear-tui/main.go` writes the list view directly without clearing to end of line. When items are rendered and the user scrolls, text from previously rendered (longer) items remains on screen because the terminal buffer isn't cleared.

This is the same root cause as the branches TUI rendering artifact issue fixed in [[tickets/1770084000_md_branches_tui_list_items_not_extending.md|md branches tui list items not extending]].

---

## Execution Plan
1. ~~Read the current View() function in linear-tui/main.go~~
2. ~~Modify the list rendering to add clear-to-end-of-line codes after each line~~
3. ~~Rebuild the linear-tui binary~~
4. ~~Test with tmux to verify no rendering artifacts~~

---

## Commands Run / Actions Taken

### Code Changes
File: `/Users/davidson/workspace/linear-tui/main.go`

Changed the list rendering in `View()` function (line 754-755) to add clear-to-end-of-line codes:

Before:
```go
// Render active list
if m.activeTab < len(m.lists) {
    b.WriteString(m.lists[m.activeTab].View())
}
```

After:
```go
// Render active list with clear-to-end-of-line codes to prevent ghost text
if m.activeTab < len(m.lists) {
    clearEOL := "\033[K"
    listView := m.lists[m.activeTab].View()
    listLines := strings.Split(listView, "\n")
    for i := range listLines {
        listLines[i] = listLines[i] + clearEOL
    }
    b.WriteString(strings.Join(listLines, "\n"))
}
```

### Build
```bash
cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .
```

---

## Results

**FIXED** - The TUI now renders cleanly when navigating with j/k keys:
- No ghost text artifacts
- No duplicated headers
- No horizontal text blending
- Tab switching works cleanly
- Rapid scrolling works cleanly

Before: Characters got corrupted when scrolling, with text from previous items appearing horizontally on the same line.

After: Clean rendering with all text properly displayed on separate lines, no artifacts.

---

## Verification Commands / Steps

### 1. Initial View Test (100% passed)
```bash
tmux new-session -d -s linear-test -x 120 -y 40 "/Users/davidson/workspace/linear-tui/linear-tui"
sleep 5 && tmux capture-pane -t linear-test -p | head -30
```
Result: Items display correctly with proper formatting.

### 2. Scroll Down Test (100% passed)
```bash
for i in {1..5}; do tmux send-keys -t linear-test 'j'; sleep 0.3; done
tmux capture-pane -t linear-test -p | head -30
```
Result: Scrolling works, no ghost text artifacts.

### 3. Scroll Up Test (100% passed)
```bash
for i in {1..5}; do tmux send-keys -t linear-test 'k'; sleep 0.3; done
tmux capture-pane -t linear-test -p | head -30
```
Result: Scrolling back up works, no artifacts.

### 4. Rapid Scroll Test (100% passed)
```bash
for i in {1..10}; do tmux send-keys -t linear-test 'j'; sleep 0.15; done
for i in {1..10}; do tmux send-keys -t linear-test 'k'; sleep 0.15; done
```
Result: Even rapid scrolling shows no rendering artifacts.

### 5. Tab Switching + Scroll Test (100% passed)
```bash
tmux send-keys -t linear-test '2'  # Switch to Planned tab
for i in {1..5}; do tmux send-keys -t linear-test 'j'; sleep 0.2; done
```
Result: Tab switching and scrolling on different tabs works cleanly.

### 6. Stress Test (100% passed)
```bash
for i in {1..5}; do
    tmux send-keys -t linear-test '1'; sleep 0.2
    tmux send-keys -t linear-test 'j'; sleep 0.2
    tmux send-keys -t linear-test '2'; sleep 0.2
    tmux send-keys -t linear-test 'j'; sleep 0.2
done
```
Result: Rapid tab switching with scrolling shows no artifacts.

### Verification Summary
- Initial view: **Verified**
- Scroll down: **Verified**
- Scroll up: **Verified**
- Rapid scrolling: **Verified**
- Tab switching: **Verified**
- Stress test: **Verified**

**Verification: 100% complete**

---

## Bugs Encountered
None - the fix was successful and all functionality works as expected.

---

## Additional Verification (2026-02-02)

Independently verified the fix with actual terminal capture evidence:

1. **Code change confirmed** - Grep verified the ANSI clear-to-end-of-line codes are present in `/Users/davidson/workspace/linear-tui/main.go`

2. **End-to-end tmux tests with captured output:**
   - Initial view: Clean rendering, JAY-190 selected
   - Scroll down 5x (j key): Selection moved to JAY-140, no ghost text artifacts
   - Scroll up 5x (k key): Selection returned to JAY-190, clean rendering
   - Stress test (rapid scroll + tab switch): Switched to Planned tab, selected JAY-141, no corruption

3. **Comparison to original bug:**
   - Before fix: Horizontal text blending like `│ JAY-190: ...review             JAY-185: Geocoder revert...`
   - After fix: All items render on separate lines, no ghost text

**Verification: PASSED** - The ANSI clear-to-end-of-line fix successfully eliminates rendering artifacts during navigation.
