Date: 2026-02-01
Title: Headless Color Mode Flag
Status: completed
Dependencies: None

## Description
Headless color mode should only be enabled with a special flag. By default, headless mode should not include ANSI color codes.

## Original Request
> Headless color mode should only be enabled with a special flag, by default we should not use color mode in headless

## Implementation
Added `--headless-color` flag to enable ANSI color codes in headless mode HTTP responses.

Usage:
```bash
./als-manager --headless                      # No colors (default)
./als-manager --headless --headless-color     # With ANSI colors
```

When enabled, the `/view/raw` endpoint includes ANSI escape codes for colors.

## Commands Run / Actions Taken
1. Added `headlessColor` flag in main.go
2. Updated `RunHeadless()` signature to accept `enableColor bool`
3. Made `lipgloss.SetColorProfile(termenv.TrueColor)` conditional on flag

## Results
- Default headless mode: No ANSI color codes
- With `--headless-color`: Full ANSI color support in `/view/raw`

## Verification Commands / Steps
```bash
# Without color flag - should have no ANSI codes
./als-manager --headless &
curl -s 'http://localhost:9877/view/raw' | xxd | grep "1b 5b"  # Empty

# With color flag - should have ANSI codes
./als-manager --headless --headless-color &
curl -s 'http://localhost:9877/view/raw' | xxd | grep "1b 5b"  # Matches
```

### Verification completed: 100%
