#!/bin/zsh

export TERM=xterm

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR" || exit 1

TICKETS_DIR="$SCRIPT_DIR/tickets"

# Capture arguments passed to wiggums (as array to preserve separation)
CLAUDE_ARGS=("$@")

echo "Using tickets directory: $TICKETS_DIR"
if [ ${#CLAUDE_ARGS[@]} -gt 0 ]; then
  echo "Claude args: ${CLAUDE_ARGS[*]}"
fi

trap 'exit 130' INT

while :; do
  if [ ! -d "$TICKETS_DIR" ]; then
    echo "Error: No tickets directory found"
    exit 1
  fi

  # Verify recently completed tickets
  # Find completed tickets whose YAML frontmatter status is NOT "completed + verified"
  recently_completed=""
  for f in $(find "$TICKETS_DIR" -name "*.md" -not -name "CLAUDE.md" -mmin -60 -exec grep -li "status: completed" {} + 2>/dev/null); do
    # Extract only the YAML frontmatter (between --- delimiters) and check status
    header_status=$(awk '/^---$/{n++; next} n==1{print} n==2{exit}' "$f" | grep -i "status:")
    echo "$header_status" | grep -qi "completed + verified" || recently_completed="$recently_completed $f"
  done
  recently_completed=$(echo "$recently_completed" | xargs)

  if [ -n "$recently_completed" ]; then
    echo "Running prompts/verify.md"
    echo "$recently_completed" | xargs -n1 basename
    {
      sed "s|{{WIGGUMS_DIR}}|$SCRIPT_DIR|g" "./prompts/verify.md"
      echo ""
      echo "## Tickets to verify:"
      for f in $recently_completed; do
        echo "- $f"
      done
    } | claude "${CLAUDE_ARGS[@]}"
    exit_code=$?
    [ $exit_code -eq 0 ] && exit 0
    # Delay before retry on failure to prevent rapid looping
    echo "Claude exited with code $exit_code, retrying in 5s..."
    sleep 5 || exit 130
    continue
  fi

  remaining=$(grep -riL "status: completed" --include="*.md" --exclude="CLAUDE.md" "$TICKETS_DIR")

  if [ -z "$remaining" ]; then
    echo "✅ All tasks completed, checking again in 10s..."
    sleep 5 || exit 130
    continue
  fi

  echo "⏳ Remaining:"
  echo "$remaining" | xargs -n1 basename

  echo "Running prompts/prompt.md"
  {
    sed "s|{{WIGGUMS_DIR}}|$SCRIPT_DIR|g" "./prompts/prompt.md"
    echo ""
    echo "## Remaining tickets:"
    echo "$remaining" | while read -r f; do
      echo "- $f"
    done
  } | claude "${CLAUDE_ARGS[@]}"
  exit_code=$?
  [ $exit_code -eq 0 ] && exit 0
  # Delay before retry on failure to prevent rapid looping
  echo "Claude exited with code $exit_code, retrying in 5s..."
  sleep 5 || exit 130
done
