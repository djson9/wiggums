#!/bin/zsh

export TERM=xterm

cd "$(dirname "$0")" || exit 1

trap 'exit 130' INT

while :; do
  # Verify recently completed tickets and bugs
  recently_completed=$(find "./bugs" "./tickets" -name "*.md" ! -name "CLAUDE.md" -mmin -3 -exec grep -li "status: completed" {} + 2>/dev/null | xargs grep -L "completed + verified" 2>/dev/null)
  
  if [ -n "$recently_completed" ]; then
    echo "Running verify.md"
    cat "./verify.md" | claude "$@"
    [ $? -eq 0 ] && exit 0
    continue
  fi

  remaining=$(grep -riL "status: completed" --include="*.md" --exclude="CLAUDE.md" "./bugs" "./tickets")
  
  if [ -z "$remaining" ]; then
    echo "✅ All tasks completed, checking again in 10s..."
    sleep 5
    continue
  fi
  
  echo "⏳ Remaining:"
  echo "$remaining" | xargs -n1 basename
  
  echo "Running prompt.md"
  cat "./prompt.md" | claude "$@"
  [ $? -eq 0 ] && exit 0
done
