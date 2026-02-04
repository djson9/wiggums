Date: 2026-01-31
Title: Verify Clip Count Detection for Already-Bounced Audio Tracks
STATUS: COMPLETED
Dependencies: 1769879588_Already_Bounced_Audio_Track_Detection.md (bug fix)

Description:
Verify that the clip count change detection fix for already-bounced audio tracks works correctly. The concern raised is that "bounces can bounce down to more than one clip" - if true, this would break the detection logic which assumes clip count goes from N > 1 to exactly 1.

## Investigation Plan

1. **Understand Ableton's bounce behavior via OSC**:
   - Use `am track inspect <uuid>` to examine audio tracks before/after bounce
   - Check what clip data is available via OSC
   - Verify if bounce always consolidates to 1 clip or can result in multiple

2. **Review the current implementation**:
   - Read `checkBounceCompleted()` in `ableton-manager-cli/handlers/bounce.go`
   - Understand the clip count detection logic

3. **Test with real tracks**:
   - Find audio tracks with multiple clips
   - Test bounce and observe clip count changes
   - Document any edge cases

4. **Document findings and update bug status**:
   - If fix is verified, update verify.md with results
   - If issues found, document and propose solution

## Findings

### Ableton Bounce Behavior Confirmed

Tested with real tracks in Ableton via OSC:

1. **Bounces with gaps DO NOT consolidate to 1 clip** - Ableton preserves the clip structure:
   - KSHMR track: 28 clips → 28 clips after bounce (with gaps between clips)
   - nostalgic pluck: 9 clips → still had multiple clips after bounce

2. **Clip names get "(Bounce)" suffix added to each clip**:
   - "KSHMR_Snare_Enhancer_19_Knock (Bounce) (Bounce) (Bounce) (Bounce)" → "(Bounce) (Bounce) (Bounce) (Bounce) (Bounce)"
   - "Serum 10 (Bounce)" → "Serum 10 (Bounce) (Bounce)"

### Why the Fix Works Anyway

The `checkBounceCompleted()` function uses 3 detection methods in order:

1. **Method 1: Clip count change** - `originalClipCount > 1 && currentClipCount == 1`
   - Does NOT trigger for tracks with gaps (28 clips stays 28 clips)
   - Only works for tracks with contiguous clips that consolidate to 1

2. **Method 2: Clip name change** - Compares first clip name before/after
   - DOES trigger correctly when "(Bounce)" suffix is added
   - This is the method that actually detects bounce completion for already-bounced tracks

3. **Method 3: File path change** - Fallback for single-clip tracks

### Test Results

| Track | Original Clips | After Bounce | Detection Method | Result |
|-------|---------------|--------------|------------------|--------|
| 29-KSHMR_Snare_Enhancer_18_Knock | 28 | 28 | Clip name change | ✓ Success (6.7s) |
| nostalgic pluck | 9 | 9 | Clip name change | ✓ Success (12.9s) |

### Conclusion

The concern in verify.md is **technically valid** - bounces CAN result in multiple clips (not just 1). However, the fix **still works correctly** because:

1. Method 1 (clip count) falls through when clips don't consolidate
2. Method 2 (clip name change) catches the bounce via the "(Bounce)" suffix addition

The bug documentation slightly misrepresents the fix - it's not primarily clip count detection that saves already-bounced tracks, it's the clip name change detection (Method 2). But the overall fix is working as intended.
