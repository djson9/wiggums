while :; do
  # Verify recently completed tickets and bugs
  recently_completed=$(find "$plan_dir/bugs" "$plan_dir/tickets" -name "*.md" ! -name "CLAUDE.md" -mmin -3 -exec grep -li "status: completed" {} + 2>/dev/null | xargs grep -L "completed + verified" 2>/dev/null)
  
  if [ -n "$recently_completed" ]; then
    cat "$plan_dir/verify.md" | claude "$@"
    continue
  fi

  remaining=$(grep -riL "status: completed" --include="*.md" --exclude="CLAUDE.md" "$plan_dir/bugs" "$plan_dir/tickets")
  
  if [ -z "$remaining" ]; then
    echo "All tasks completed, checking again in 10s..."
    sleep 10
    continue
  fi
  
  echo "⏳ Remaining:"
  echo "$remaining" | xargs -n1 basename
  
  cat "$plan_dir/prompt.md" | claude "$@"
done
