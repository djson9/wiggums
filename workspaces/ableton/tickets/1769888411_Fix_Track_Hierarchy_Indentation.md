Date: 2026-01-31
Title: Fix Track Hierarchy Indentation Bug
Status: COMPLETED
Dependencies: None

## Original Description
It appears that verse plucks is under Verse Chords, but it's not indented properly, it looks like they're siblings.

## Root Cause
The `parent_group_index` field in the CLI uses `omitempty` JSON tag. When a track's parent is index 0, Go omits the field from JSON output (0 is the zero value). The React frontend then can't determine the parent-child relationship.

## Implementation

1. Fixed `models/track.go:33` - removed `omitempty` from ParentGroupIndex
2. Fixed `models/responses.go:144` - removed `omitempty` from ParentGroupIndex
3. Fixed `handlers/tracks.go:382` - initialized ParentGroupIndex to -1 in TrackListItem
4. Added defensive check in `TrackTree.tsx:67` - handles undefined parent_group

## Verification

CLI output now shows correct hierarchy:
```json
{"index": 0, "name": "Verse [nb]", "parent_group_index": -1}
{"index": 1, "name": "Verse Chords", "parent_group_index": 0}  // Was missing, now present!
{"index": 2, "name": "verse plucks", "parent_group_index": 1}
```

## Comments

2026-01-31 18:53: Picked up ticket. Created plan: 1769888411_Plan_Fix_Track_Hierarchy_Indentation.md
- Identified root cause: omitempty on parent_group_index in models/track.go
- Track 1 "Verse Chords" has parent at index 0 (Verse [nb]), but 0 is omitted
- React frontend gets undefined for parent_group, causing incorrect indentation

2026-01-31 19:00: Implemented fix
- Fixed 4 files: models/track.go, models/responses.go, handlers/tracks.go, TrackTree.tsx
- Build passes (make build), tests pass (make test)
- React app builds (npm run build)
- CLI output verified: parent_group_index: 0 now appears for tracks under index 0
- Verification: 85% - CLI data verified correct. Remaining 15%: Manual UI test in browser
