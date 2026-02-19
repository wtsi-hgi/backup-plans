# Phase 4: Caching and config integration

Ref: [spec.md](spec.md) sections B2, C2, C3, C1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

## Items

### Item 4.1: B2 - Caching fofn status

spec.md section: B2

Extend `MultiCache` in `ibackup/ibackup.go` with a
`GetFofnBackupActivity` method that looks up the correct
`FofnStatusReader` by path regex, calls `GetBackupActivity`, and
caches the result. Also create supporting code in
`ibackup/fofnstatus.go` as needed. Write tests in
`ibackup/fofnstatus_test.go` covering all 3 acceptance tests from
spec.md section B2.

- [ ] implemented
- [ ] reviewed

### Batch 2 (parallel, after item 4.1 is reviewed)

#### Item 4.2: C2 - Fofndir-only server handling in New() [parallel with 4.3]

spec.md section: C2

Modify `createServers()` in `ibackup/ibackup.go` to skip
`connect()` when `Addr` is empty. Store a nil `serverClient` for
fofndir-only servers. Ensure `MultiClient.Backup()` skips API
operations when the underlying client is nil. Write tests in
`ibackup/ibackup_test.go` covering all 4 acceptance tests from
spec.md section C2.

- [ ] implemented
- [ ] reviewed

#### Item 4.3: C3 - Config parsing for FofnDir field [parallel with 4.2]

spec.md section: C3

Add `FofnDir` field to `ServerDetails` in `config/config.go`.
Implement `GetFofnDirs()` method on `Config` that returns a map
of path-regexp to fofndir for all servers that have a fofndir
configured. Write tests in `config/config_test.go` covering all
3 acceptance tests from spec.md section C3.

- [ ] implemented
- [ ] reviewed

For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill (review
all items in the batch together in a single review pass).

### Item 4.4: C1 - Creating fofn files during backup

spec.md section: C1

Extend `backups.Backup` in `backups/backups.go` to call
`FofnDirWriter.Write` for directories whose path matches a server
with fofndir configured. Handle both API+fofn and fofndir-only
scenarios. Also handle config-only updates when the FOFN write is
frequency-gated but the requestor has changed. Wire the fofn writer
into the backup flow in `cmd/backup.go`. Depends on items 4.2 and
4.3 for fofndir-only server support and config parsing. Write
tests in `backups/backups_test.go` covering all 7 acceptance tests
from spec.md section C1.

- [ ] implemented
- [ ] reviewed
