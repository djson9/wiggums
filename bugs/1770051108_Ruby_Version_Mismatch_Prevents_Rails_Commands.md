---
Date: 2026-02-02
Title: Ruby Version Mismatch Prevents Rails Commands
Status: completed
Severity: low
Description: Cannot run Rails commands in middesk repo due to Ruby version mismatch between system (3.4.7) and project requirement (3.4.6).
---

## Reproduction Steps

1. Navigate to `/Users/davidson/workspace/middesk`
2. Run any Rails command, e.g.: `bin/rails generate migration TestMigration`

## Error Message

```
Your Ruby version is 3.4.7, but your Gemfile specified 3.4.6 (Bundler::RubyVersionMismatch)
```

## Expected Behavior
Rails commands should execute successfully with the correct Ruby version.

## Actual Behavior
All Rails commands fail with RubyVersionMismatch error.

## Environment
- System Ruby: 3.4.7 (`/opt/homebrew/Cellar/ruby/3.4.7/`)
- Required Ruby: 3.4.6 (per Gemfile)
- chruby: Installed but Ruby 3.4.6 not available

## User Impact
- Cannot generate migrations locally
- Cannot run `bin/rails db:migrate`
- Cannot run Rails console
- Cannot run specs locally

## Files to Explore
- `/Users/davidson/workspace/middesk/Gemfile` - Ruby version requirement
- `/Users/davidson/workspace/middesk/.ruby-version` - Ruby version file
- `~/.rubies/` - chruby Ruby installations

## Resolution

Install Ruby 3.4.6 using ruby-install:
```bash
ruby-install 3.4.6 -- --enable-shared
source /opt/homebrew/opt/chruby/share/chruby/chruby.sh
chruby 3.4.6
```

Or update Gemfile to allow 3.4.7 (not recommended - should match CI/CD environment).

## Related Tickets
- [[tickets/1770051004_Add_County_Migration_To_AddressLookups.md|Add County Migration To AddressLookups]] - Blocked from full verification by this bug
