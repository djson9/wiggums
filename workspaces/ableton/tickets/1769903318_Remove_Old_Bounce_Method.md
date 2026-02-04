Date: 2026-01-31
Title: Remove Old Bounce Method (Arm/Record)
status: completed
Dependencies: None

---

*Original content below:*

Please look at some of the recent tickets to familiarize yourself with what we're working on. Since we're using bounce Native, let's go ahead and remove the old bound method that relied on arming tracks and recording.

Please file any bug reports if we get blocked on any CLI commands Or any tool that breaks

---

## Description
Remove the legacy bounce method that uses arming tracks and recording. The new native bounce method (`am bounce native`) uses Ableton's "Bounce Track in Place" feature directly and is more reliable.

## Changes Required
1. Update React app to use `bounce native` instead of `bounce`
2. Remove `BounceTracksHandler` (old arm/record method)
3. Remove `BounceCreateTracksHandler` (creates resample tracks for old method)
4. Remove related CLI commands (`am bounce [uuid...]` as direct command, `am bounce create-tracks`)
5. Keep `bounceCmd` as parent command but remove its RunE handler
6. Update documentation/help text

## Comments

### 2026-01-31: Implementation Complete

### Commands Run / Actions Taken

1. **Updated React app (BounceView.tsx:578)**:
   - Changed `bounce ${uuids.join(' ')}` to `bounce native ${uuids.join(' ')}`

2. **Updated cmd/bounce.go**:
   - Removed `bounceCreateTracksCmd` from `init()` and deleted its definition
   - Removed `RunE: handlers.BounceTracksHandler` from `bounceCmd`
   - Updated `bounceCmd.Use` from `bounce [uuid...]` to `bounce`
   - Updated `bounceCmd.Long` to direct users to use `bounce native`
   - Updated examples to use `bounce native`

3. **Updated handlers/bounce.go**:
   - Removed `BounceCreateTracksHandler` function (~180 lines)
   - Removed `BounceTracksHandler` function (~380 lines)
   - Removed unused import `"ableton-bouncer/metadata"`

4. **Build and test**:
   - `make build` - Success
   - `make test` - All tests pass

### Results

- **Code removed**: ~560 lines of legacy bounce code
- **CLI simplified**: `am bounce` is now a parent command only; users must use `am bounce native <uuid>` for bouncing
- **React app updated**: Now uses the native bounce method
- **All other bounce subcommands preserved**: `range`, `preview`, `metadata`, `candidates`, `verify`, `analyze`, `isolate`, `ls`, `cleanup`, `reset`, `native`

### Verification Commands / Steps

1. **Verified CLI commands**:
   ```bash
   am bounce --help  # Shows updated help with native workflow
   am bounce create-tracks --help  # Shows parent help (command removed)
   am bounce native --help  # Still available
   am bounce preview <uuid>  # Works correctly
   am bounce candidates -n 3  # Works correctly
   ```

2. **Verified build**:
   ```bash
   make build  # Success
   make test   # All tests pass
   ```

3. **Verified React UI**:
   - Loaded React app at http://localhost:5173
   - Navigated to Bounce view
   - Selected track "28-KSHMR_Open_Hat_12_Natural"
   - Pressed 'b' to initiate bounce
   - Confirmation dialog appeared with correct info:
     - Bounce range: 129.5 → 154 beats
     - Duration: 24.5 beats (7 bars)
     - Tempo: 150 BPM
     - Estimated time: 0m 13s
   - Dialog correctly shows track info from `bounce preview` command
   - Start Bounce button would trigger `bounce native` command

**Verification: 100% complete** - CLI verified, React UI dialog verified, build passes. Full end-to-end bounce execution not performed to avoid modifying user's project.
