trap 'exit 0' INT

cd "$(dirname "$0")" || exit 1

while :; do
  # Verify recently completed tickets and bugs
  recently_completed=$(find "./bugs" "./tickets" -name "*.md" ! -name "CLAUDE.md" -mmin -3 -exec grep -li "status: completed" {} + 2>/dev/null | xargs grep -L "completed + verified" 2>/dev/null)
  
  if [ -n "$recently_completed" ]; then
    cat "./verify.md" | claude "$@"
    continue
  fi

  remaining=$(grep -riL "status: completed" --include="*.md" --exclude="CLAUDE.md" "./bugs" "./tickets")
  
  if [ -z "$remaining" ]; then
    echo "All tasks completed, checking again in 10s..."
    sleep 3
    continue
  fi
  
  echo "⏳ Remaining:"
  echo "$remaining" | xargs -n1 basename
  
  cat "./prompt.md" | claude "$@"
done
