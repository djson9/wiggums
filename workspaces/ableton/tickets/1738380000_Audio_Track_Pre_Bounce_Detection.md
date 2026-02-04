Date: 2026-01-31
Title: Audio Track Pre-Bounce Detection - Skip Already-Bounced Tracks
Status: in_progress
Dependencies: 1769913307_Audio_Track_Bounce_Investigation.md (completed)
Description: Detect audio tracks that won't change BEFORE bouncing, so we can skip them immediately instead of waiting 60 seconds for the timeout.

---
*Original content:*

Continued from
tickets/1769913307_Audio_Track_Bounce_Investigation.md

Investigate whether we can detect audio files that won't change BEFORE we bounce.

If so, implement.

---

## Investigation Summary

From ticket 1769913307_Audio_Track_Bounce_Investigation.md:
- Pure audio tracks with clips that reference external audio files don't change during bounce
- "Bounce Track in Place" for these tracks produces NO changes because there's nothing to render
- Current solution: 60-second timeout with "no_change_detected" status

## Detection Criteria

Definitive indicators that an audio track is "already bounced" and won't change:

1. **Clip name contains "(Bounce)"** - Ableton adds this suffix to clip names when bouncing
2. **File path contains "/Bounce/"** - Bounce files are stored in the project's Bounce folder

Additional indicators (lower confidence):
3. **Single clip with external audio file reference AND no effect devices** - Nothing to render

## Implementation Plan

Modify `BounceNativeHandler` in `ableton-manager-cli/handlers/bounce.go`:

1. Add helper function `isAlreadyBounced(clipNames []string, clipPaths []string) bool`
   - Returns true if ANY clip name contains "(Bounce)"
   - Returns true if ANY file path contains "/Bounce/"

2. In the bounce loop, after getting clip paths/names but BEFORE triggering bounce:
   - For audio tracks, call `isAlreadyBounced()`
   - If true, skip bounce with status "already_bounced" and success=true
   - Record in workflow as "skipped" status

3. Benefits:
   - Saves 60 seconds per already-bounced audio track
   - Clearer status message ("already_bounced" vs "no_change_detected")
   - Faster batch processing

---

## Commands Run / Actions Taken

### 1. Initial Investigation
Found that `isAlreadyBounced()` function already existed but wasn't being reached for tracks with many clips due to a timeout issue.

```bash
# Found track with bounced clips
am tracks --detailed | jq -r '.tracks[] | select(.arrangement_clips) | select([.arrangement_clips[].name] | any(contains("(Bounce)"))) | "\(.uuid[:8]): \(.name)"'
# Result: 1902d999: main vox chops

# Verified clip names contain "(Bounce)"
am track inspect 1902d999 | jq '.track.arrangement_clips[0].name'
# Result: "output (25) [2023-12-30 215250] (Bounce)"

# Initial bounce test - FAILED
am bounce native 1902d999-98f8-4771-a764-9747443125a8 --verbose
# Result: "track 'main vox chops' has no clips to bounce" (INCORRECT)
```

### 2. Root Cause Analysis
Traced through the code and found:
1. `osc.GetArrangementClipFilePaths(index)` times out for tracks with many clips (193 clips)
2. This causes `clipPaths` to be empty
3. The check `if len(clipPaths) == 0` at line 1100 triggers "no clips to bounce" error
4. The `isAlreadyBounced()` check at line 1171 is never reached

```bash
# Direct gRPC test confirmed timeout
cd ableton-manager-cli && go run /tmp/test_paths.go
# Result: "rpc error: code = Unknown desc = failed to get file paths (batch offset=0): timeout waiting for response"

# But raw OSC works
ao raw /live/track/get/arrangement_clips/file_path_batch 81 0 10
# Result: Returns file paths with "/Bounce/" in them
```

### 3. Implementation
Modified `ableton-manager-cli/handlers/bounce.go`:

1. Added new helper function `isAlreadyBouncedByName(clipNames []string)` that only checks clip names (line 1838-1845)
   - More reliable than checking file paths which can timeout

2. Reordered the bounce validation logic:
   - Get `clipCount` first using `GetNumArrangementClips()` (reliable)
   - Get `clipNames` for already-bounced detection
   - Check `isAlreadyBouncedByName()` BEFORE the "no clips" check
   - Use `clipCount` instead of `len(clipPaths)` for the "no clips" check

3. Updated `originalClipCount` in `tracksToBounce` struct to use `clipCount` instead of `len(clipPaths)`

### 4. Build and Test
```bash
make build
# Result: Build successful

am bounce native 1902d999-98f8-4771-a764-9747443125a8 --verbose
# Result: "Audio track 'main vox chops' is already bounced (detected via clip names) - skipping"
# Status: "already_bounced", success: true

# Test non-bounced track still proceeds to bounce
am bounce native 6abec8e6-2711-4923-91e6-d549aa362a12 --verbose
# Result: "Bouncing track 1/1: reverse cymbal" - correctly proceeds to bounce flow

make test
# Result: All tests pass
```

---

## Results

### Key Changes to `handlers/bounce.go`:
1. **New function `isAlreadyBouncedByName(clipNames []string)`** - Detects already-bounced tracks using only clip names (lines 1838-1845)
2. **Moved already-bounced check earlier** - Now runs BEFORE the "no clips" check (lines 1098-1129)
3. **Use `clipCount` for clip count validation** - More reliable than `len(clipPaths)` which can timeout (line 1137)
4. **Use `clipCount` for `originalClipCount`** - Ensures bounce completion detection works correctly (line 1233)

### Benefits:
- Tracks with "(Bounce)" in clip names are skipped **immediately** (0 seconds)
- Previously: Would either timeout at 60s or fail with "no clips" error
- Works even when file path fetching times out (common for tracks with many clips)

---

## Verification Commands / Steps

### Test 1: Already-bounced track detection
```bash
am bounce native 1902d999-98f8-4771-a764-9747443125a8 --verbose
```
Expected: `status: "already_bounced"`, `success: true`
Actual: ✅ Passed

### Test 2: Non-bounced track proceeds to bounce
```bash
am bounce native 6abec8e6-2711-4923-91e6-d549aa362a12 --verbose
```
Expected: Track proceeds to bounce flow ("Bouncing track 1/1...")
Actual: ✅ Passed - Shows "Bounce triggered via menu item"

### Test 3: All tests pass
```bash
make test
```
Actual: ✅ All tests pass

### Verification Percentage
**100% verified** - End-to-end testing completed:
- [x] Already-bounced audio track detected via clip names
- [x] Non-bounced audio track proceeds to normal bounce
- [x] Build succeeds
- [x] All tests pass

---

status: completed

