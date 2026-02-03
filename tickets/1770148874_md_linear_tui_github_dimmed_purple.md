---
Date: 2026-02-03
Title: Linear TUI GitHub Links - Dimmed Purple When Not Focused
Status: completed + verified
Description: Change GitHub link colors to use dimmed purple when not focused and regular purple when focused for better visual hierarchy.
---

## Original Request
[[tickets/1770138257_md_linear_tui_github_links.md|Linear TUI GitHub Links]]
Can we show a dimmed purple when not focused and regular purple when focused

## Related Tickets
- [[tickets/1770138257_md_linear_tui_github_links.md|GitHub Links Implementation]] - original GitHub links feature
- [[tickets/1770136630_md_linear_tui_dim_comments_and_spacing.md|Dim comments]] - similar dim color patterns
- [[tickets/1770092188_md_linear_tui_color_and_tab_numbers.md|Color patterns]] - color scheme standards

## Execution Plan
1. Read current styles in `/Users/davidson/workspace/linear-tui/styles.go`
2. Change `githubURLStyle` to use dimmed purple (non-focused state)
3. Change `selectedGithubURLStyle` to use regular/bright purple (focused state)
4. Build the project
5. Verify in tmux that colors differ between focused and non-focused

## Additional Context
- Current state: Both `githubURLStyle` and `selectedGithubURLStyle` use color "141" (same purple)
- Current `selectedGithubURLStyle` only adds grey background (236), no color change
- 256-color palette reference:
  - 135 = bright magenta/purple
  - 141 = light purple (current)
  - 99 = medium-dim purple
  - 97 = darker purple

## Commands Run / Actions Taken

1. **Read current styles**: Examined `/Users/davidson/workspace/linear-tui/styles.go`
   - Found both `githubURLStyle` and `selectedGithubURLStyle` used color "141"
   - `selectedGithubURLStyle` only added grey background (236), no color differentiation

2. **Modified styles.go**:
   ```go
   // Before:
   githubURLStyle = lipgloss.NewStyle().
       Foreground(lipgloss.Color("141")) // subtle purple
   selectedGithubURLStyle = lipgloss.NewStyle().
       Background(selectedBg).
       Foreground(lipgloss.Color("141"))

   // After:
   githubURLStyle = lipgloss.NewStyle().
       Foreground(lipgloss.Color("97"))  // dimmed purple
   selectedGithubURLStyle = lipgloss.NewStyle().
       Background(selectedBg).
       Foreground(lipgloss.Color("141")) // regular purple
   ```

3. **Built project**: `cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .` - success

4. **Verified in tmux**: Tested color output by navigating between items

## Results

- Non-focused GitHub links now display in dimmed purple (color 97)
- Focused GitHub links display in regular purple (color 141) with grey background
- Visual hierarchy is now clear - you can easily tell which item is focused by the brighter purple

## Verification Commands / Steps

1. **Launched TUI in tmux**:
   ```bash
   tmux new-session -d -s linear-test -x 120 -y 30 "cd /Users/davidson/workspace/linear-tui && ./linear-tui"
   ```

2. **Captured ANSI codes when viewing item 1** (item 2's GitHub link is non-focused):
   ```
   ^[[38;5;97m  https://github.com/middesk/middesk/pull/15172^[[39m
   ```
   - Color 97 (dimmed purple) confirmed

3. **Navigated to item 2** (GitHub link is now focused):
   ```bash
   tmux send-keys -t linear-test 'j'
   ```
   Captured:
   ```
   ^[[38;5;141m^[[48;5;236m  https://github.com/middesk/middesk/pull/15172^[[39m^[[49m
   ```
   - Color 141 (regular purple) with background 236 (grey) confirmed

4. **Navigated back to item 1** to confirm non-focused returns to dimmed:
   - Color 97 confirmed again

**Verification: 100% complete**
- Code change implemented and builds successfully
- Non-focused GitHub links use dimmed purple (97) - verified via ANSI codes
- Focused GitHub links use regular purple (141) - verified via ANSI codes
- Visual difference is clear and matches request

