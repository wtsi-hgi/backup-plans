# Phase 1: Foundation - ibackup dependency and types

Ref: [spec.md](spec.md) sections A1

## Instructions

Implement the user stories referenced below, strictly following
the TDD cycle described in spec.md. For each acceptance test in
the spec, write a corresponding GoConvey test. Do not skip,
stub out, or circumvent any test. Do not hardcode expected
results in implementations to make tests pass artificially. All
tests must genuinely pass.

Check the "implemented" checkbox when you believe the item is
complete, then stop. A separate reviewer will verify your work
and check the "reviewed" checkbox. Only after review passes may
remaining items be implemented.

Use the `orchestrator` skill to complete this phase, coordinating
subagents with the `go-implementor` and `go-reviewer` skills.

## Items

### Item 1.1: A1 - Filesystem-safe set name conversion

spec.md section: A1

Upgrade `go.mod` to ibackup v1.16.0. Implement the `SafeName`
function in `ibackup/fofndir.go` that strips the `plan::` prefix
and replaces every `/` (U+002F) with fullwidth solidus (U+FF0F).
Write tests in `ibackup/fofndir_test.go` covering all 5
acceptance tests from spec.md section A1.

- [ ] implemented
- [ ] reviewed
