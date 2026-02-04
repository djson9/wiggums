# Bounce Inspect and LS Improvements

Date: 2026-02-01
Title: Add am bounce inspect [uuid] and improve am bounce ls
status: completed

## Description
1. Add `am bounce inspect [uuid]` command to see detailed bounce runs for a specific track UUID
   - Show time created for each bounce
   - Show time completed for each bounce

2. Update `am bounce ls` to show:
   - Total time (overall duration from first queued to last completed)
   - Time created (when the workflow was created)

## Original Request
Can we also add an am bounce inspect [uuid] so that we can see the detailed runs for a uuid? So what time created for each bounce, what time completed?

Also am bounce ls should show total time, and time created.

## Plan
1. Add `am bounce inspect [uuid]` subcommand in cmd/bounce.go
2. Add handler for inspect in handlers/bounce.go that queries workflows by track_uuid
3. Update `am bounce ls` output to include total elapsed time per batch

## Commands Run / Actions Taken
1. Ran `am bounce ls -n 5` - verified batch_summary includes `created_at`, `completed_at`, and `total_elapsed_seconds`
2. Ran `am bounce inspect 17dd4477-9ba7-436d-af28-18ff9cf372c6` - verified output includes `created_at`, `completed_at`, `duration_seconds` for each bounce, and `total_duration_seconds`
3. Verified `am bounce inspect --help` shows correct documentation
4. Verified `am bounce ls --help` mentions batch summary timing fields

## Results
Both features were already implemented in previous work:

**`am bounce inspect [uuid]`** (handlers/bounce.go:1929-2022):
- ✅ `created_at`: Time created for each bounce
- ✅ `started_at`: Time started for each bounce
- ✅ `completed_at`: Time completed for each bounce
- ✅ `duration_seconds`: Duration of each bounce
- ✅ `total_duration_seconds`: Sum of all bounce durations
- ✅ `count`, `completed_count`, `failed_count`: Statistics

**`am bounce ls`** (handlers/bounce.go:2024-2163):
- ✅ `created_at` in batch_summary: When the batch was first queued
- ✅ `completed_at` in batch_summary: When the batch last completed
- ✅ `total_elapsed_seconds` in batch_summary: Total elapsed time from first queued to last completed

## Verification Commands / Steps
```bash
# Verify am bounce ls shows timing in batch_summary
am bounce ls -n 5 | jq '.batch_summary'

# Verify am bounce inspect shows timing for each bounce
am bounce inspect 17dd4477-9ba7-436d-af28-18ff9cf372c6 | jq '.bounces[] | {created_at, completed_at, duration_seconds}'

# Verify help documentation
am bounce inspect --help
am bounce ls --help
```

**Verification Status: 100% complete**
- All required fields are present in CLI output
- Help documentation accurately describes features
- End-to-end verification with real bounce data confirmed
