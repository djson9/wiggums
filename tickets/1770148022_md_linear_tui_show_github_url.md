---
Date: 2026-02-03
Title: Show GitHub URL in md linear tui for issues with PRs
Status: completed + verified
Description: Display GitHub PR URL in linear tui to indicate the 'g' shortcut is available to open it
---

For md linear tui I see a github link is on
```
 1. JAY-185: Geocoder revert reverted changes in Middesk
    https://linear.app/jays-work-desk/issue/JAY-185/geocoder-revert-reverted-changes-in-middesk
      [Feb 2 5:26pm]: Opened PR for adding to middesk
      [Feb 2 5:20pm]: Final test

```

I would just like to see the github url somewhere, so it would indicate to us that we are able to press g to go that url. Maybe in like a subtle purple color

## Related Tickets
- [[tickets/1770138257_md_linear_tui_github_links.md|Linear TUI GitHub Links]] - implemented "g" key handler and GetGitHubURL() method

## Execution Plan
1. Add a subtle purple style in `styles.go` for GitHub URLs
2. Modify `issue.go` Description() method to include GitHub URL if available
3. Modify delegate in `main.go` to apply purple styling to GitHub URL lines
4. Build and verify in tmux

## Additional Context
- The GetGitHubURL() method already exists in api.go:58
- The "g" key handler already works
- We just need to DISPLAY the URL visually so users know it's available

## Commands Run / Actions Taken
1. **Modified `styles.go`** (lines 51-58):
   - Added `githubURLStyle` with subtle purple color (141)
   - Added `selectedGithubURLStyle` with purple color on grey background

2. **Modified `issue.go`** (lines 36-39):
   - Added GitHub URL to `Description()` method with 2-space indent
   - Uses existing `GetGitHubURL()` method from api.go

3. **Modified `main.go`** (lines 118-128):
   - Updated delegate's Render() to detect GitHub URL lines
   - Applies purple style for lines containing "github.com"
   - Uses `selectedGithubURLStyle` when selected, `githubURLStyle` otherwise

4. **Built project**: `(cd /Users/davidson/workspace/linear-tui && go build -o linear-tui .)`

## Results
- GitHub URLs now display in subtle purple (color 141) below the Linear URL
- Only appears for issues that have a GitHub attachment (e.g., linked PRs)
- Visual indicator tells user that 'g' shortcut is available
- Purple styling maintained when item is selected (with grey background)

Display example:
```
 2. JAY-185: Geocoder revert reverted changes in Middesk
    https://linear.app/jays-work-desk/issue/JAY-185/geocoder-revert-reverted-changes-in-middesk
      https://github.com/middesk/middesk/pull/15172  <-- purple
```

## Verification Commands / Steps
1. **Build verification**: Project builds with no errors

2. **TUI test in tmux**:
```bash
tmux new-session -d -s linear-test -x 120 -y 30 "cd /Users/davidson/workspace/linear-tui && ./linear-tui"
sleep 2 && tmux capture-pane -t linear-test -p -e
```

3. **Verified GitHub URL shows for JAY-185**:
   - ANSI code `[38;5;141m` (purple) applied to GitHub URL line
   - Other description lines use `[38;5;246m` (grey)

4. **Verified selected state styling**:
   - Navigated to item 2 with 'j' key
   - GitHub URL shows `[38;5;141m[48;5;236m` (purple on grey background)
   - ✅ `selectedGithubURLStyle` working correctly

5. **Verified "g" shortcut still works**:
   - On item with GitHub URL: Opens browser (no error message)
   - On item without GitHub URL: Shows "No GitHub link found" in status bar
   - ✅ Existing functionality preserved

**Verification: 100% complete**
- Code compiles: ✅
- GitHub URL displays: ✅
- Purple color applied: ✅
- Selected state styling: ✅
- "g" shortcut works: ✅
- "No link" message works: ✅

## Additional Verification (2026-02-03)
**Independent verification confirmed original results:**
- Built linear-tui from source: compiled successfully
- Ran TUI in tmux (140x35) and captured pane output with ANSI codes
- Confirmed JAY-185 shows GitHub URL `https://github.com/middesk/middesk/pull/15172`
- ANSI code `[38;5;141m` (purple color 141) applied to GitHub URL line ✅
- Other description lines use `[38;5;246m` (grey color 246) as expected
- Feature working as intended - visual indicator for 'g' shortcut present
