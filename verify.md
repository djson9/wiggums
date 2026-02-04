# Verify

## Verifying Existing Tickets
Please run:
```
find "{{WIGGUMS_DIR}}/tickets" -name "*.md" -not -name "CLAUDE.md" -mmin -60 -exec grep -li "status: completed" {} + 2>/dev/null | xargs grep -L "completed + verified" 2>/dev/null
```

If any results comes back, these are tickets that were recently completed. Now please determine:
- Did the verification steps actually verify what the user asked for? Did we successfully test end to end? Here are examples of good and bad verification:
Bad:  Verified only that dialog appears, not actually running the command
Bad: Verified only that help menu appears, not actually running the command
Bad: Verified only that code looks correct, not actually running the command
Bad: Verified only that the backend works, not that the frontend displays it 
Bad: Verified only that the command ran, not that the output changed
Bad: Checked state once after action without comparing to state before  
Good: Verifying that the state changed in the expected manner
Good: Running the command end to end
Good: Capture TUI output after action
Good: Verify specific text changed (e.g., ⏹ became ▶, 00:00 became 00:05)
- If no, please investigate further by debugging our tooling, any instrumentation, or the feature itself.
- If we need to debug
	- Add logging statements liberally throughout the codebase to trace execution flow
	- Place logs at multiple levels: entry/exit points of functions, before/after critical operations, inside loops, and at error boundaries
	- Log both up the call stack and deep into implementation details
	- Include relevant variable values, state information, and timestamps in logs
	- After debugging, please note your "Commands Run / Actions Taken", "Results", "Verification Commands / Steps".

If a ticket looks properly verified, update the status to "completed + verified", so that it no longer matches our grep. Add notes if you made any additional verification.

If we make any code changes, do NOT update the status. Instead, add repro steps to exactly verify the results. Another agent will come after you and perform the repro steps, and will update the status if they are able to repro.

Please look and work on at ONLY ONE ticket at a time. We should modify at MOST one ticket at a time.
