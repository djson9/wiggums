# Investigation: Group Tracks with Children Clips Should Be Bounceable

**Ticket:** 1769901547_Verify_Batch_Bounce_All_Fixes.md
**Date:** 2026-01-31

## Investigation Question
> Can we check if tracks with no clips have children and if we should still be bouncing?

## Findings

### Current Behavior (Before Fix)
In `bounce.go:1661-1691`, tracks were skipped if `len(clipPaths) == 0`:
```go
if len(clipPaths) == 0 {
    errMsg := fmt.Sprintf("track '%s' has no clips to bounce", trackName)
    // ... skip track
}
```

This was WRONG for group tracks because:
1. Group tracks don't have clips directly on them (clips are on child tracks)
2. `GetArrangementClipFilePaths(index)` returns 0 for groups even when children have clips
3. When Ableton bounces a group track, it bounces all audio from children (through the group's processing chain)

### Data Available
From `am tracks --detailed`:
- Group tracks have `clip_count: null` (no direct clips)
- Group tracks have `group_clip_regions` computed from children clips
- Example: "Verse [nb]" has 0 clips but `group_clip_regions` shows 142+ clips from children

### OSC Methods Available
- `GetTrackIsGroup(index)` - Check if track is a group
- `GetTrackGroupIndex(index)` - Get parent group index for a track
- `GetNumArrangementClips(trackIndex)` - Get clip count for a track

## Solution Design

For group tracks with 0 direct clips:
1. Iterate all tracks
2. For each track, check if parent group index matches our group index
3. If any child has clips (via GetNumArrangementClips), allow bounce to proceed
4. If no children have clips, skip with appropriate message

## Commands Run / Actions Taken

### 1. Investigation
```bash
am tracks --detailed | jq '[.tracks[] | select(.is_group == true) | select(.group_clip_regions | length > 0)] | .[0:3]'
# Found group tracks with children clips: "Verse [nb]", "Verse Chords", "verse plucks"

am track inspect 5fd49  # Verified "verse plucks" is a group with no direct clips
```

### 2. Implementation
Added helper function `groupHasChildrenWithClips()` to `bounce.go` at line 2239:
- Iterates all tracks
- Checks if each track's parent group index matches the group
- Recursively checks nested groups
- Returns true if any child has clips

Modified skip logic at line 1661:
- Non-group tracks with 0 clips: skip (unchanged)
- Group tracks with 0 clips: check children
  - If children have clips: proceed with bounce
  - If no children have clips: skip with appropriate message

### 3. Testing
```bash
make build  # Success

# Test group track bounce
am bounce native 5fd49 --verbose
# Output:
# "Group at index 2 has child at index 3 with 21 clips"
# "Group track 'verse plucks' has no direct clips but children have clips - proceeding with bounce"
# (Track NOT skipped - fix working)
# Bounce fails later: "Can't get menu item "Bounce Track in Place"" (Ableton limitation)

# Test regular track bounce
am bounce native 2e005 --verbose
# Output: "Bounce completed in 12.1s" - Success

make test  # All tests pass
```

## Results

**Fix implemented successfully:**
1. Added `groupHasChildrenWithClips()` helper function
2. Modified skip logic to check children for group tracks
3. Group tracks with children are no longer incorrectly skipped

**New limitation discovered:**
- Ableton doesn't have "Bounce Track in Place" menu for group tracks
- Filed bug report: `1769903131_Group_Track_Bounce_Menu_Not_Available.md`

## Verification Commands / Steps

| Test | Command/Action | Expected | Actual | Status |
|------|----------------|----------|--------|--------|
| Group track not skipped | `am bounce native 5fd49 --verbose` | "proceeding with bounce" message | Message shown, not skipped | PASS |
| Children detected | Verbose output | Shows "child at index X with Y clips" | Shows "child at index 3 with 21 clips" | PASS |
| Regular track bounce | `am bounce native 2e005` | Bounce succeeds | Bounced in 12.1s | PASS |
| Build succeeds | `make build` | No errors | Build successful | PASS |
| Tests pass | `make test` | All pass | All pass | PASS |

**Verification: 100%** - Fix implemented and verified. Group tracks with children are no longer skipped.

**Note:** Actual group track bouncing is blocked by Ableton limitation (no "Bounce Track in Place" for groups). Filed as separate bug.

status: completed
