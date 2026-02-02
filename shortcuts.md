# Shortcuts - Iteration Learnings

## Investigating Middesk PRs and Incidents
- Use `gh pr view <PR#> --repo middesk/middesk` to view PR summary
- Use `gh pr diff <PR#> --repo middesk/middesk` to see code changes
- Use `gh pr view <PR#> --repo middesk/middesk --comments` for discussion context
- Use `git log --oneline -20 -- <filepath>` in `/Users/davidson/workspace/middesk` to trace related commits
- Use `git show <commit>` to see full diffs of specific commits

## Schema/Migration Investigation Pattern
When investigating schema issues:
1. Read the model file for schema annotation (top of file shows columns)
2. Grep for existing migrations: `create_table|add_column|change_table.*<table_name>`
3. Look at similar recent migrations for the correct format/Rails version

## Creating Migrations in Middesk

**Preferred method:** `cd /Users/davidson/workspace/middesk && bin/rails generate migration MigrationName column:type`

**If Ruby version mismatch:** Manually create migration file:
1. Get timestamp: `date +"%Y%m%d%H%M%S"`
2. Create file: `db/migrate/TIMESTAMP_snake_case_name.rb`
3. Use `ActiveRecord::Migration[8.0]` (check recent migrations for current version)
4. Follow existing patterns (see `db/migrate/` for examples)

**About structure.sql:** Auto-updated when migrations run (`config.active_record.schema_format = :sql`). No manual edits needed.

**Ruby version requirement:** Use `nix develop` for correct Ruby version:
```bash
cd /Users/davidson/workspace/middesk
nix develop --command bin/rails db:migrate
nix develop --command bin/rails db:migrate:status
nix develop --command bundle install  # if gems missing
```
