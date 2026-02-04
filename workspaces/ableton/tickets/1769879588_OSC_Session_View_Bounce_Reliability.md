Date: 2026-01-30
Title: OSC Session View Bounce Reliability
STATUS: COMPLETED
Dependencies: None

Description:
Bounce automation was unreliable because track selection via OSC in Arrangement View doesn't enable the "Bounce Track in Place" menu. The solution was to programmatically switch to Session View via OSC before selecting tracks and triggering bounce.

Changes Made:
1. Added generic OSC API handlers to AbletonOSC Remote Script:
   - `/live/api/call <path> [args...]` - Call methods on LOM objects
   - `/live/api/get <path>` - Get properties from LOM objects
   - `/live/api/set <path> <value>` - Set properties on LOM objects
   - This allows any Live Object Model operation without reloading the Remote Script

2. Updated `switchToSessionView()` in bounce.go:
   - Old: AppleScript Tab key toggle (unreliable, state-dependent)
   - New: `ao raw /live/api/call application.view.focus_view Session` (programmatic, deterministic)

3. Added `GetAOPath()` helper in shared/file_utils.go:
   - Finds `ao` binary relative to `am` executable
   - Fallback to hardcoded path if not found

Verification:
Tested with 6 tracks of different types:
- 98-Serum (MIDI) - completed in 16.1s
- nostalgic pluck (MIDI) - completed in 19.6s
- knowing pluck (MIDI) - completed in 20.8s
- wonder pluck (MIDI) - completed in 20.6s
- reflections texture (MIDI) - completed in 27.4s
- karra vocal reverse (audio) - completed in 19.4s

Result: 6/6 successful bounces (100% success rate)

Comments:
2026-01-30 20:25: All MIDI and fresh audio tracks bounce correctly. One edge case found with already-bounced audio tracks (see bug report).
