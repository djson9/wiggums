# Shortcuts - Iteration Learnings

## Investigating Middesk PRs and Incidents
- Use `gh pr view <PR#> --repo middesk/middesk` to view PR summary
- Use `gh pr diff <PR#> --repo middesk/middesk` to see code changes
- Use `gh pr view <PR#> --repo middesk/middesk --comments` for discussion context
- Use `git log --oneline -20 -- <filepath>` in `/Users/davidson/workspace/middesk` to trace related commits
- Use `git show <commit>` to see full diffs of specific commits

## PR Review Comments
- Use `md pr comments middesk <PR#>` for middesk/middesk repo
- Use `md pr comments middesk/geocoder <PR#>` for other repos (full org/repo format)
- Use `gh pr view <PR#> --repo <org/repo> --json reviews,comments,state,reviewDecision` for full PR status
- Use `gh pr checks <PR#> --repo <org/repo>` to check CI status

## Finding PRs by Branch Name
- Use `gh pr list --repo <org/repo> --head <branch-name> --json number,title,url`
- Use `gh pr list --repo <org/repo> --search "<keywords>" --json number,title,url,headRefName` for keyword search

## Viewing Remote File Contents (without cloning)
- Use `gh api "repos/<org>/<repo>/contents/<path>" --jq '.content' | base64 -d` for main branch
- Use `gh api "repos/<org>/<repo>/contents/<path>?ref=<branch>" --jq '.content' | base64 -d` for specific branch
- Pipe to `| sed -n '<start>,<end>p'` to view specific line ranges

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

## Investigating Skyvern/Agent Tasks
Agent commands require a running middesk console pod. Create one first:
```bash
md kube pod new middesk          # Creates pod, shows pod name
md kube pod ls                   # List running pods
md kube pod kill <pod-name> -n production  # Cleanup when done
```

**Viewing specific agent tasks:**
```bash
md agent task show <task-uuid>
```

**Skyvern workflow reports (requires pod):**
```bash
md agent skyvern retrieval fl_department_of_revenue --limit-days 14
md agent skyvern submission ca_employment_development_department --limit-days 7
```

**Viewing Skyvern run details:**
```bash
md skyvern runs <run_id>         # Get full output including extracted_information
md skyvern workflows             # List all workflows
```

**Linear issue investigation:**
```bash
md linear show AGSUP-2685        # Get full issue details as JSON
md linear oncall ls              # List oncall queue
```

## Key Skyvern Files in Middesk
- Base retrieval logic: `/Users/davidson/workspace/middesk/app/jobs/agent/skyvern_workflows/base_retrieval.rb`
- Agency configs: `/Users/davidson/workspace/middesk/app/jobs/agent/skyvern_workflows/<agency>_retrieval_config.rb`
- Constants (account IDs): `/Users/davidson/workspace/middesk/app/lib/agent/constants.rb`
- The `account_status` (success/processing/remediation/duplicate) comes from Skyvern extraction, not Ruby code
