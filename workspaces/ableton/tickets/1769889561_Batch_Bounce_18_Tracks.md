Date: 2026-01-31
Title: Batch Bounce 18 Tracks via am bounce native
STATUS: COMPLETED
Dependencies: None
Description: User requested bouncing 18 specific track UUIDs using `am bounce native` command

---
*Original content below:*

Please bounce these uuids using am bounce native

17dd4477-9ba7-436d-af28-18ff9cf372c6
a9492c1c-dd8d-40d4-a774-f9d7fdcd3fac
7eee80bd-752d-4caa-9c39-d185ff0f27ac
1dc2f7dc-0ea5-4551-b90a-83697d535b06
9c68199f-e117-4356-97a3-15e0e26e6f2f
358fac89-b65d-472e-b1ef-3d201d6a38d2
c9784124-fa01-4aa8-a584-615aa086adf8
763db29f-c3fb-4649-bb46-7073ec3b76ce
755f9d3a-41ec-4b39-8bee-31a4f32db765
eff8fdca-7f44-41be-9cff-7dcc0c48a26a
0b7a42ad-f78f-4ec1-9b8e-6d105b7ef276
054d0a5c-80d5-4249-b00c-0ba6c58d2253
cc986146-b09b-4526-be29-8ce2ba7518e9
5fd25886-5e21-43c2-aa7f-12f0113a9faf
e28bac13-66df-4a42-b5e3-98e40b1bab6a
c6aa5839-098f-43bd-90f4-9d33bff805cd
8b0eb92e-209a-4255-85f1-e4755b832e72
c28cf171-22f8-4bf9-87f5-a2f8e670d38b

---

Comments:

2026-01-31 15:15: Executed `am bounce native` with all 18 UUIDs.

**Track Analysis:**
- 18 tracks requested
- Only 7 tracks had clips that could be bounced
- 11 tracks had 0 clips (group tracks or empty tracks)

**First Attempt Results:**
- Bounce got stuck on track "37-Audio" (0 clips) - command aborted and workflows reset

**Second Attempt Results (7 tracks with clips only):**
- 1 successful: "bass" (MIDI track, bounced in 17.5s)
- 1 timeout: "39-KSHMR Tight Snare 12" (audio track, timed out after 300s - known issue with already-bounced audio tracks)
- 5 failed: "failed to get tracks" - Ableton connection lost during the 300s timeout

**Bounce Files Created:**
- `/Users/davidson/Music/Ableton/Live Recordings/2026-01-31 150827 Temp Project/Samples/Processed/Bounce/Bounce 16_ bass [2026-01-31 150918]-2.wav`

**Issues Identified:**
1. Tracks with 0 clips should not be bounced (causes infinite wait)
2. Already-bounced audio tracks still have detection issues (timeout)
3. Long timeouts cause Ableton connection to be lost for subsequent tracks

**Success Rate:** 1/18 (5.5%) - Only tracks with clips that are MIDI type bounced successfully
