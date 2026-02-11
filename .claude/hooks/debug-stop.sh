#!/bin/bash
# Debug hook: log Stop event JSON input to inspect available fields
INPUT=$(cat /dev/stdin)
echo "$INPUT" | jq . >> /tmp/claude-stop-debug.log
echo "---" >> /tmp/claude-stop-debug.log
