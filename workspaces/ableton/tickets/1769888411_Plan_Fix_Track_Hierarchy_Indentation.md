Date: 2026-01-31
Title: Plan - Fix Track Hierarchy Indentation Bug
Status: COMPLETED
Dependencies: None

## Description
Track "verse plucks" appears at same indentation level as "Verse Chords" despite being a child of it.

## Root Cause Analysis

1. In `models/track.go:33`:
   ```go
   ParentGroupIndex int  `json:"parent_group_index,omitempty"`
   ```

2. When a track is a child of index 0 (like "Verse Chords" under "Verse [nb]"), `ParentGroupIndex` is 0

3. Go's `omitempty` treats 0 as the zero value and OMITS it from JSON output

4. The React frontend receives no `parent_group_index` field, so `parent_group` becomes `undefined`

5. In TrackTree.tsx, the check `track.parent_group < 0` doesn't catch `undefined` (since `undefined < 0` is `false`)

6. The track gets treated as top-level, missing its tree prefix indentation

## Solution

Fix in CLI (models/track.go): Use a pointer `*int` for ParentGroupIndex so 0 can be distinguished from "not set":
```go
ParentGroupIndex *int `json:"parent_group_index,omitempty"` // nil = no parent, 0+ = valid parent index
```

OR remove `omitempty`:
```go
ParentGroupIndex int  `json:"parent_group_index"` // Always output, -1 = no parent
```

The second option is simpler and more explicit. We'll use -1 to indicate no parent (root level).

Also add a defensive fix in React (TrackTree.tsx) to handle undefined properly:
```typescript
if (track.depth === 0 || track.parent_group == null || track.parent_group < 0) return '';
```

## Implementation Plan

1. Edit models/track.go - remove omitempty from ParentGroupIndex
2. Ensure handlers set ParentGroupIndex = -1 for root tracks
3. Add defensive check in TrackTree.tsx for undefined parent_group
4. Build and test

### Commands Run / Actions Taken
1. Edited models/track.go:33 - removed `omitempty`
2. Edited models/responses.go:144 - removed `omitempty`
3. Edited handlers/tracks.go:375-382 - added `ParentGroupIndex: -1` initialization
4. Edited TrackTree.tsx:67 - added null check: `track.parent_group == null`
5. Ran `make build` - passed
6. Ran `make test` - passed
7. Ran `npm run build` in React app - passed

### Results
CLI now outputs correct `parent_group_index` for all tracks:
- Track 0 (Verse [nb]): parent_group_index: -1 (root)
- Track 1 (Verse Chords): parent_group_index: 0 (FIXED - was missing)
- Track 2 (verse plucks): parent_group_index: 1 (correct)

### Verification Commands / Steps
1. Run `am tracks --detailed | jq '.tracks[0:3] | .[] | {name, parent_group_index}'`
2. Verify "Verse Chords" (index 1) shows parent_group_index: 0 ✓
3. Open React app and verify "verse plucks" is indented under "Verse Chords"

**Verification Status: 85%**
- CLI data verified correct
- Build/tests pass
- Remaining 15%: Manual UI test in browser to confirm visual indentation
