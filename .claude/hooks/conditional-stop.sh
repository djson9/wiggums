#!/bin/bash
cat /dev/stdin | "$CLAUDE_PROJECT_DIR"/wiggums hook-helpers should-stop
if [ $? -eq 10 ]; then
  kill -9 $PPID
fi
exit 0
