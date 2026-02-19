# Phase 3: Config update and status reading

Ref: [spec.md](spec.md) sections A4, B1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

## Items

### Batch 1 (parallel)

#### Item 3.1: A4 - Config-only update for claim transfer [parallel with 3.2]

spec.md section: A4

Implement `FofnDirWriter.UpdateConfig` in `ibackup/fofndir.go`.
This method rewrites only the config.yml in the fofn subdirectory
for the given set name without touching the FOFN file. Write tests
in `ibackup/fofndir_test.go` covering all 2 acceptance tests from
spec.md section A4.

- [ ] implemented
- [ ] reviewed

#### Item 3.2: B1 - Parsing fofn status files [parallel with 3.1]

spec.md section: B1

Implement `FofnStatusReader` struct, `NewFofnStatusReader`
constructor, and `GetBackupActivity` method in
`ibackup/fofnstatus.go`. The method reads a `status` file from
the subdirectory named by `SafeName(setName)`, calls
`fofn.ParseStatus` from the ibackup library, and maps the result
to a `SetBackupActivity`. Write tests in
`ibackup/fofnstatus_test.go` covering all 4 acceptance tests from
spec.md section B1.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill (review
all items in the batch together in a single review pass).
