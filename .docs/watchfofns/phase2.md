# Phase 2: FOFN directory writing

Ref: [spec.md](spec.md) sections A2, A3

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

## Items

### Item 2.1: A2 - Writing FOFN files and config

spec.md section: A2

Implement `FofnDirWriter` struct, `NewFofnDirWriter` constructor,
and the `Write` method in `ibackup/fofndir.go`. The method creates
a subdirectory named by `SafeName(setName)` under baseDir, writes
a null-terminated FOFN file, and writes config.yml using
`fofn.WriteConfig` from the ibackup library. Write tests in
`ibackup/fofndir_test.go` covering all 4 acceptance tests from
spec.md section A2.

- [ ] implemented
- [ ] reviewed

### Item 2.2: A3 - Frequency gating for FOFN writes

spec.md section: A3

Extend `FofnDirWriter.Write` in `ibackup/fofndir.go` with
frequency gating logic: if the FOFN file's mtime plus
`(frequency-1)*24h + 12h` is after `time.Now()`, skip the write.
For frozen sets (frequency == 0), skip if the FOFN file already
exists. Write tests in `ibackup/fofndir_test.go` covering all 7
acceptance tests from spec.md section A3. Depends on item 2.1
for the basic Write functionality.

- [ ] implemented
- [ ] reviewed
