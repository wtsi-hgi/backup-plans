# Phase 6: Integration tests

Ref: [spec.md](spec.md) sections E1

## Instructions

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

## Items

### Item 6.1: E1 - End-to-end backup with fofn files

spec.md section: E1

Add integration tests in `main_test.go` that verify the full
backup-plans backup command creates fofn files alongside ibackup
sets. Test with both API+fofndir and fofndir-only configurations.
Also verify that the server's summary endpoint returns fofn status
from a pre-created status file. Depends on all prior phases. Cover
all 3 acceptance tests from spec.md section E1.

- [ ] implemented
- [ ] reviewed
