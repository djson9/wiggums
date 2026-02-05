# Debugging wiggums.sh

**Status:** Resolved

## Issue: Script looping repeatedly running verify.md

### Symptoms
- Script kept printing "Running verify.md" and spawning new Claude sessions
- Each Claude session reported "No tickets found that need verification"
- Loop continued indefinitely

### Root Causes Found

#### 1. Timing Race Condition (`-mmin -3`)
The script used `-mmin -3` (files modified in last 3 minutes) to find recently completed tickets. However:
1. Script finds file at T+2 minutes (2 < 3, matches)
2. Script spawns Claude
3. Claude takes time to start up
4. Claude runs the same check at T+4 minutes (4 > 3, no match)
5. Claude reports nothing found
6. Script continues loop

**Fix:** Changed `-mmin -3` to `-mmin -60` (1 hour window)

#### 2. Claude's Working Directory Mismatch
Even though the script does `cd "$(dirname "$0")"`, Claude Code spawned via pipe inherits the *user's original working directory*, not the script's current directory. This caused:
1. Script runs `find ./bugs ./tickets` from `/Users/davidson/workspace/cli-middesk/wiggums/` → finds files
2. Claude runs the same command from user's original cwd (e.g., `~/workspace`) → finds nothing
3. Script loops because it keeps finding files but Claude reports nothing

**Fix:** Use `sed` substitution to inject absolute paths into prompts before piping to Claude:
```bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
sed "s|{{WIGGUMS_DIR}}|$SCRIPT_DIR|g" "./verify.md" | claude "$@"
```

And use `{{WIGGUMS_DIR}}` placeholder in prompt files:
```bash
find "{{WIGGUMS_DIR}}/bugs" "{{WIGGUMS_DIR}}/tickets" ...
```

#### 3. Bash History Expansion (`!` operator)
The `!` operator in `find ... ! -name "CLAUDE.md"` triggers bash history expansion when combined with pipes, causing `find: \!: unknown primary or operator` errors.

**Fix:** Use `-not` instead of `!`:
```bash
# Before (broken in bash with pipes)
find ... -name "*.md" ! -name "CLAUDE.md" ... | xargs ...

# After (works)
find ... -name "*.md" -not -name "CLAUDE.md" ... | xargs ...
```

### Debugging Steps Used
```bash
# 1. Check what files exist with "status: completed"
grep -ri "status: completed" --include="*.md" ./bugs ./tickets

# 2. Check file modification times
ls -la ./tickets/*.md ./bugs/*.md

# 3. Test find command in isolation
find "./bugs" "./tickets" -name "*.md" -not -name "CLAUDE.md" -mmin -60

# 4. Test find + grep step
find ... -exec grep -li "status: completed" {} +

# 5. Test full pipeline
find ... | xargs grep -L "completed + verified"

# 6. Verify variable capture works
recently_completed=$(find ... | xargs ...); echo "Found: '$recently_completed'"
```

### Files Modified
- `wiggums.sh` - Added `SCRIPT_DIR`, changed `cat` to `sed` substitution, changed `!` to `-not`, changed `-mmin -3` to `-mmin -60`
- `verify.md` - Changed paths to `{{WIGGUMS_DIR}}/...` placeholders, changed `!` to `-not`, changed `-mmin -3` to `-mmin -60`
- `prompt.md` - Changed paths to `{{WIGGUMS_DIR}}/...` placeholders

### Lessons Learned
1. When a script spawns a subprocess that runs the same check, ensure timing constraints are generous enough for startup latency
2. Child processes spawned via pipe may NOT inherit the parent's working directory - use absolute paths or template substitution
3. Use `sed` substitution with placeholders like `{{VAR}}` to inject runtime values into prompts
4. Avoid `!` in find commands when piping through bash - use `-not` instead to prevent history expansion issues
5. Debug pipelines step-by-step: test each stage in isolation before combining
