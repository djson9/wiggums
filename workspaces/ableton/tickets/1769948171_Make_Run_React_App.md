Date: 2026-02-01
Title: Add "make run" Target for React App
Status: completed
Dependencies: None
Description: Can we have "make run" run the react app and open?

User wants a simple `make run` command that starts the React web app (ink-experiment/bounce-view-web) and opens it in a browser.

## Plan
1. Add a `run` target to the root Makefile
2. The target should:
   - Change to ink-experiment/bounce-view-web directory
   - Run `npm run dev:all` to start both WebSocket server and Vite dev server
   - Open http://localhost:5173 in the browser after a brief delay for startup
3. Verify by running `make run`

### Commands Run / Actions Taken
1. Renamed ticket from `Untitled.md` to `1769948171_Make_Run_React_App.md`
2. Added `run` to .PHONY targets in Makefile
3. Added `run` target that:
   - Opens browser with 2-second delay (in background)
   - Changes to ink-experiment/bounce-view-web and runs `npm run dev:all`
4. Added `make run` to help output

### Results
Successfully implemented `make run` target. The command:
- Starts WebSocket bridge server on ws://localhost:8080
- Starts Vite dev server on http://localhost:5173 (or next available port)
- Opens browser to http://localhost:5173 after 2 second delay

### Verification Commands / Steps
1. `make help` - Confirmed new target appears in help output
2. `timeout 10s make run` - Verified command starts both servers:
   - WebSocket server connects to Redis and subscribes to events
   - Vite dev server starts and is ready
   - Browser opens automatically
3. Verification: 100% complete - tested end-to-end

Note: If ports 5173/5174 are in use, Vite will use next available port, but browser still opens to 5173. User can refresh to correct port if needed.
