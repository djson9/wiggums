Date: 2026-02-01
Title: Kill Existing Processes Before Make Run
Status: completed
Dependencies: 1769948171_Make_Run_React_App.md (completed)
Description: When running `make run`, terminate existing processes on ports 5173/5174/8080 instead of trying alternate ports.

## Problem
Following from [1769948171_Make_Run_React_App](1769948171_Make_Run_React_App.md), when ports are in use:
```
[1] Port 5173 is in use, trying another one...
[1] Port 5174 is in use, trying another one...
```

User wants existing processes killed so the app always starts on port 5173, rather than incrementing to find an available port.

## Plan
1. Modify the `run` target in root Makefile
2. Before starting the dev servers, kill any processes on:
   - Port 5173 (Vite dev server - primary)
   - Port 5174 (Vite dev server - fallback)
   - Port 8080 (WebSocket bridge server)
3. Use `lsof` to find and `kill` processes on these ports
4. Verify the fix works

### Commands Run / Actions Taken
1. Renamed ticket from `Untitled.md` to `1769948636_Kill_Existing_Processes_Before_Make_Run.md` with proper metadata
2. Modified Makefile `run` target (lines 74-83) to add port-killing commands:
   - Added `lsof -ti :5173 | xargs kill -9` for primary Vite port
   - Added `lsof -ti :5174 | xargs kill -9` for fallback Vite port
   - Added `lsof -ti :8080 | xargs kill -9` for WebSocket server port
   - Added `sleep 0.5` to ensure processes are fully terminated before starting
   - Used `@-` prefix and `|| true` to suppress errors when no processes found

### Results
Successfully modified Makefile. The `run` target now:
- Kills any existing processes on ports 5173, 5174, and 8080 before startup
- Displays "Killing any existing processes on ports 5173, 5174, 8080..." message
- Proceeds to start Vite and WebSocket servers after a brief delay

### Verification Commands / Steps
1. **Port blocking test**: Started Python HTTP server on port 5173 to simulate stale process
2. **Ran `make run`**: Verified output shows:
   - "Killing any existing processes on ports 5173, 5174, 8080..."
   - Vite starts on `http://localhost:5173/` (NOT an alternate port like 5174/5175)
3. **Confirmed Vite output**: No "Port 5173 is in use, trying another one..." message appeared

Verification: 100% complete - tested end-to-end with blocking process simulation
