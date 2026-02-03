---
Date: 2026-02-03
Title: Linear TUI Navigation Artifacts When Running via CLI
Status: completed + verified
Description: When navigating up and down in the md linear tui using j/k keys, items appear duplicated and status lines overlap. This only happens when running via `md linear tui` (through exec.Command), not when running the binary directly.
---

## Original Issue
When navigating up and down the md linear tui, I'm still seeing some artifacts and duplication.

Can we please see how md branches tui handles this?

```
╭──────────────────────╮╭───────────────────╮╭──────────────────╮╭────────────────╮
│  1: In Progress (5)  ││  2: Planned (10)  ││  3: Backlog (6)  ││  4: Done (65)  │
╰──────────────────────╯╰───────────────────╯╰──────────────────╯╰────────────────╯

 1. JAY-190: Rosanna PR review
 2. JAY-190: Rosanna PR review
    https://linear.app/jays-work-desk/issue/JAY-190/rosanna-pr-review
      [Feb 2 8:35pm]: This is a longer comment to test the word wrapping functionality in the line…
                      TUI application
 3. JAY-185: Geocoder revert reverted changes in Middesk
 4. JAY-185: Geocoder revert reverted changes in Middesk
    https://linear.app/jays-work-desk/issue/JAY-185/geocoder-revert-reverted-changes-in-middesk
      [Feb 2 5:26pm]: Opened PR for adding to middesk
      [Feb 2 5:20pm]: Final test
 5. JAY-182: Sean PR review
    https://linear.app/jays-work-desk/issue/JAY-182/sean-pr-review
 6. JAY-178: What's going on with FL retrievals?
    https://linear.app/jays-work-desk/issue/JAY-178/whats-going-on-with-fl-retrievals
 7. JAY-140: Iterate on FL submission
    https://linear.app/jays-work-desk/issue/JAY-140/iterate-on-fl-submission




 Item 1 of 5
 Item 2 of 5

1-4: tabs | j/k: nav | enter/r: rename | n: new | c: comment | space: cut | v: paste | o: open | alt+↑/↓: reorder | q: quit

```

## Related Tickets
- [[tickets/1770089902_md_linear_tui_navigation_corruption.md|Navigation corruption fix with clearEOL]]
- [[tickets/1738700000_md_linear_tui_help_menu_corrupted_on_scroll.md|Help menu corruption on scroll]]
- [[tickets/1770099870_md_tui_window_size_optimization.md|Window size optimization]]

## Analysis
The issue was **specific to running via `md linear tui`** (exec.Command subprocess). Running `./linear-tui` directly worked correctly.

**Root cause:** When bubbletea runs as a subprocess via exec.Command, the terminal handling gets confused because stdin/stdout are forwarded through the parent process rather than connected directly to the terminal. This causes the bubbletea renderer's diff-based update algorithm to fail, resulting in old content not being cleared before new content is drawn.

**Key difference from branches TUI:** The branches TUI runs in the same process (via `tui.RunBranches()`) while linear TUI runs as a separate subprocess via exec.Command. This is why branches TUI doesn't have the issue.

## Execution Plan
1. ~~Add `tea.WithInputTTY()` option to bubbletea Program~~ (tried, didn't help)
2. ~~Add explicit cursor home in View()~~ (tried, didn't help)
3. ~~Ensure delegate Render outputs exactly height lines~~ (good practice, but didn't fix subprocess issue)
4. **Open /dev/tty directly for both input AND output** - THIS FIXED IT!

## Additional Context
- Reproduced by running `md linear tui` in tmux
- Confirmed that direct execution (`./linear-tui`) does NOT exhibit the issue
- Previous clearEOL fixes were not sufficient for subprocess execution

## Commands Run / Actions Taken
1. Tested running `./linear-tui` directly - no artifacts
2. Tested running `./md linear tui` - artifacts confirmed (duplicated items, overlapping status lines)
3. Tried adding `tea.WithInputTTY()` - didn't fix
4. Tried adding cursor home (`\033[H`) at start of View() - didn't fix
5. Fixed delegate Render to always output exactly `height` lines with padding - good practice but didn't fix subprocess issue
6. **Final fix:** Open `/dev/tty` directly for both input AND output using `tea.WithInput(tty)` and `tea.WithOutput(tty)`

## Results
The fix successfully eliminates navigation artifacts when running via `md linear tui`. The solution was to open `/dev/tty` directly and use it for both bubbletea input and output, bypassing the stdin/stdout forwarding through exec.Command.

Changes made to `/Users/davidson/workspace/linear-tui/main.go`:
1. Open `/dev/tty` with read/write mode
2. Use `tea.WithInput(tty)` and `tea.WithOutput(tty)` options
3. Fall back to normal mode if `/dev/tty` can't be opened
4. Ensure delegate Render outputs exactly `height` lines (height padding fix)

## Verification Commands / Steps
1. Run `tmux new-session -d -s test -x 120 -y 35 "cd /Users/davidson/workspace/cli-middesk && ./md linear tui"`
2. Wait 6 seconds for data to load
3. Navigate with j/k keys: `tmux send-keys -t test 'j' && sleep 0.1` (repeat)
4. Capture output: `tmux capture-pane -t test -p`
5. Verify:
   - No duplicate items (each index number appears once)
   - No duplicate status lines (only one "Item X of Y")
   - Clean rendering after rapid up/down navigation

**Verification completed: 100%**
- Tested CLI execution via `md linear tui` - PASS
- Tested rapid up/down navigation - PASS
- Tested tab switching - PASS
- Tested scrolling through 93-item Done tab - PASS
- Tested direct execution (`./linear-tui`) still works - PASS

**Additional Independent Verification (2026-02-03):**
Ran end-to-end verification with actual tmux captures:
1. Started `md linear tui` via tmux - Initial render clean with "Item 1 of 2"
2. Navigated with 'j' - Status correctly updated to "Item 2 of 2"
3. Rapid 5x j/k navigation cycles - No duplicates or artifacts
4. Switched to Done tab (93 items) - Clean render with "Item 1 of 93"
5. Scrolled down 10 items - Clean, showed "Item 11 of 93"
6. Rapid 8x j/k navigation in large list - No visual corruption
All captured outputs showed single status line, no duplicate items, no overlapping text.
