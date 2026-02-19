# Phase 5: Server and frontend integration

Ref: [spec.md](spec.md) sections D1, D2

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

## Items

### Item 5.1: D1 - Exposing fofn status in the summary endpoint

spec.md section: D1

Extend `populateIbackupStatus` in `backend/report.go` to retrieve
fofn-based backup status via `MultiCache.GetFofnBackupActivity`
for each claimed directory with BackupIBackup rules. Add the fofn
status to the BackupStatus map with a key of `"fofn:" + dir`.
Write tests in `backend/report_test.go` covering all 3 acceptance
tests from spec.md section D1.

- [ ] implemented
- [ ] reviewed

### Item 5.2: D2 - Frontend display of fofn status counts

spec.md section: D2

Extend the `BackupStatus` TypeScript type in
`frontend/src/types.ts` with optional count fields (Uploaded,
Replaced, Unmodified, Missing, Frozen, Orphaned, Warning,
Hardlink, Failures). Update `frontend/src/report.ts` to render
these counts in the report table when present. Depends on item
5.1 for the data format. Cover all 2 acceptance tests from
spec.md section D2.

- [ ] implemented
- [ ] reviewed
