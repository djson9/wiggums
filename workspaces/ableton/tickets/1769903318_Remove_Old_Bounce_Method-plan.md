# Plan: Remove Old Bounce Method (Arm/Record)
status: completed

## Overview
Remove the legacy bounce method that uses arming tracks and recording. The React app and CLI should use `am bounce native` exclusively.

## Current State Analysis

### Old Method (to remove)
- `am bounce [uuid...]` → `BounceTracksHandler` (lines 609-988 in bounce.go)
  - Creates new audio tracks for each source track
  - Sets up routing from source to bounce track
  - Arms the bounce track
  - Enables record mode
  - Starts playback
  - Waits for recording to complete
  - Disarms tracks

- `am bounce create-tracks` → `BounceCreateTracksHandler` (lines 282-460 in bounce.go)
  - Creates bounce/resample tracks (dry-run or actual)

### New Method (keep)
- `am bounce native [uuid...]` → `BounceNativeHandler`
  - Uses Ableton's "Bounce Track in Place" via keyboard shortcut
  - Much simpler and more reliable

### Other Commands to KEEP
- `am bounce range` - Still useful for calculating clip bounds
- `am bounce preview` - Used by React app for confirmation dialog
- `am bounce metadata` - Shows what metadata would be saved
- `am bounce candidates` - Lists tracks with clip bounds
- `am bounce verify` - Verifies bounced audio files
- `am bounce analyze` - Analyzes WAV files
- `am bounce isolate` - Creates isolated ALS projects
- `am bounce ls` - Lists bounce workflows
- `am bounce cleanup` - Removes old workflows
- `am bounce reset` - Removes all workflows

## Changes Required

### 1. React App (BounceView.tsx)
- Change `bounce ${uuids.join(' ')}` to `bounce native ${uuids.join(' ')}`

### 2. cmd/bounce.go
- Remove `bounceCmd.RunE = handlers.BounceTracksHandler` (line 70)
- Remove `bounceCreateTracksCmd` from init() and definition
- Update `bounceCmd.Long` description since it won't bounce directly

### 3. handlers/bounce.go
- Remove `BounceTracksHandler` function
- Remove `BounceCreateTracksHandler` function
- Keep `calculateBounds` helper (used by preview and other handlers)

## Implementation Steps
1. Update React app to use `bounce native`
2. Remove old handlers from bounce.go
3. Update cmd/bounce.go
4. Build and test
5. Verify via React app

## Verification
- Run `am bounce native <uuid>` manually
- Test bounce via React app UI
- Verify other bounce subcommands still work
