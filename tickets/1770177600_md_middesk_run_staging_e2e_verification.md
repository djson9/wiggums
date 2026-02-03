---
Date: 2026-02-03
Title: Staging Pod E2E Verification - Spin Up and Run Commands
Status: completed + verified
Description: Verify full end-to-end workflow of staging pod: spin up a new staging kube pod, run commands to it, and verify the complete flow works.
---

## Original Request
[[tickets/1770147817_md_middesk_run_staging_flag.md|Add --staging flag to md middesk run command]]
Can you make sure we can spin up a new staging kube pod and run commands to it? And then verify the full e2e of spinning it up, running commands

## Related Tickets
- [[tickets/1770147817_md_middesk_run_staging_flag.md|Add --staging flag to md middesk run command]] - parent implementation ticket

## Execution Plan
1. Check current staging pods status
2. Attempt to spin up a staging console pod using `make console-staging` (or equivalent)
3. Verify pod comes up successfully
4. Run test command via `md middesk run "1+1" --staging`
5. Verify command executes and returns expected result
6. Document any issues encountered
7. Clean up staging pod if needed

## Additional Context
- The staging infrastructure uses the same Kubernetes namespace pattern as production (`staging` vs `production`)
- Console pods are created via `bin/console-pod staging middesk rails-console` (wrapped by `make console-staging`)
- Pods have labels `user=$USER,role=console-user` that the CLI uses to find them
- The `bin/console-pod` script has an EXIT trap that deletes the pod when you exit the console - this means the pod only exists while the console is running

## Commands Run / Actions Taken

### Step 1: Initial State Check
```bash
kubectl get pods -n staging | grep -i console
# Result: No console pods found initially
```

### Step 2: Test --staging Without Pod
```bash
md middesk run "1+1" --staging
# Result: Error: no running middesk console pod found in staging. Create one with: make console-staging (in middesk repo)
```
The error message is helpful and guides users to the correct command.

### Step 3: Spin Up Staging Pod
```bash
tmux new-session -d -s staging-console -c /Users/davidson/workspace/middesk
tmux send-keys -t staging-console "nix develop --command make console-staging" Enter
sleep 10
kubectl get pods -n staging -l role=console-user
# Result: middesk-console-json-b850d9   2/2   Running   0   11s
```

### Step 4: Test Basic Command
```bash
md middesk run "1+1" --staging
# Output:
# Using pod: middesk-console-json-b850d9 (namespace: staging)
# commandDurationMs=9198
# 2
```

### Step 5: Verify Staging Environment
```bash
md middesk run "puts Rails.env; puts ENV['DATABASE_URL']&.gsub(/:[^:@]+@/, ':***@') || 'N/A'" --staging
# Output:
# Using pod: middesk-console-json-b850d9 (namespace: staging)
# staging
# postgres:***@pgbouncer/middesk-staging
```
Confirmed Rails.env is "staging" and database is "middesk-staging".

### Step 6: Test Database Query
```bash
md middesk run "User.count" --staging
# Output: 990

md middesk run "User.count" --prod
# Output: 57666
```
Staging has 990 users, Production has 57,666 users - confirms different environments.

### Step 7: Test Mutual Exclusivity
```bash
md middesk run "1+1" --prod --staging
# Output: Error: --prod and --staging are mutually exclusive
```

### Step 8: Cleanup
```bash
kubectl delete pod middesk-console-json-b850d9 -n staging
tmux kill-session -t staging-console
```

## Results

**FULL E2E VERIFIED SUCCESSFULLY**

The `md middesk run --staging` feature works correctly:

| Test Case | Expected | Actual | Pass |
|-----------|----------|--------|------|
| No pod error | Helpful error message | `no running middesk console pod found in staging. Create one with: make console-staging` | ✅ |
| Basic command | Returns `2` for `1+1` | `2` | ✅ |
| Environment check | `staging` | `staging` | ✅ |
| Database URL | Contains `staging` | `middesk-staging` | ✅ |
| Different data | Fewer users than prod | 990 vs 57,666 | ✅ |
| Mutual exclusivity | Error when both flags | `--prod and --staging are mutually exclusive` | ✅ |

## Verification Commands / Steps

### Full E2E Workflow to Reproduce:

1. **Start staging console pod** (Terminal A):
   ```bash
   cd /Users/davidson/workspace/middesk
   nix develop --command make console-staging
   # Keep this running
   ```

2. **Run staging commands** (Terminal B):
   ```bash
   # Basic test
   md middesk run "1+1" --staging

   # Verify staging environment
   md middesk run "Rails.env" --staging

   # Run actual queries
   md middesk run "User.count" --staging
   ```

3. **Cleanup**: Exit the console in Terminal A (Ctrl+D), which triggers pod deletion

### Alternative: Using tmux
```bash
# Start
tmux new-session -d -s staging-console -c ~/workspace/middesk
tmux send-keys -t staging-console "nix develop --command make console-staging" Enter
sleep 15  # Wait for pod to be ready

# Use
md middesk run "..." --staging

# Cleanup
kubectl get pods -n staging -l role=console-user  # Find pod name
kubectl delete pod <pod-name> -n staging
tmux kill-session -t staging-console
```

**Verification: 100% complete**
- All test cases pass
- Full end-to-end flow verified
- Staging environment confirmed via Rails.env and database URL
- Data isolation confirmed (different user counts)
- Error handling verified (mutual exclusivity, no-pod error)
