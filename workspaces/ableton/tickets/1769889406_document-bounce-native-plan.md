# Plan: Document am bounce native and bounce ls commands

## Ticket Reference
- File: `plan/tickets/1769889406_document-bounce-native.md`
- Description: Document how `am bounce native` works and how to view running bounces in the CLI, adding to README concisely

## Approach
1. Gather info from existing help output:
   - `am bounce native -h` - Shows it triggers Ableton's "Bounce Track In Place" via keyboard shortcut
   - `am bounce ls -h` - Shows how to list bounce workflows
2. Add a new section to `ableton-manager-cli/README.md` documenting:
   - `am bounce native` command usage
   - `am bounce ls` command for viewing bounce status
3. Keep documentation concise and match existing README style

## Commands Run / Actions Taken
1. Ran `am bounce native -h` to understand the command
2. Ran `am bounce ls -h` to understand how to view bounces
3. Read existing `ableton-manager-cli/README.md` for style reference
4. Added "### Bounce Commands" section to README at lines 302-350 including:
   - `am bounce native` documentation with examples
   - `am bounce ls` documentation with examples
   - Recommended workflow

## Results
Successfully added documentation for bounce commands to `ableton-manager-cli/README.md`:
- Added `### Bounce Commands` section between `am reconcile` and `### Daemon Management`
- Documented `am bounce native` with usage examples and behavior description
- Documented `am bounce ls` for viewing bounce status
- Included recommended workflow with `am bounce native` -> `am reconcile --apply` -> `am save minor`

## Verification Commands / Steps
1. [x] `am bounce native -h` - Verified command works and matches documentation
2. [x] `am bounce ls -h` - Verified command works and matches documentation
3. [x] Read updated README lines 302-350 - Documentation is correct, concise, and follows existing style

Verification: 100% complete - Documentation verified against actual CLI help output

status: completed

STATUS: COMPLETED