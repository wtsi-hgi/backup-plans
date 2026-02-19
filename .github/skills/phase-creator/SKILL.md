---
name: phase-creator
description: Creates phase plan documents from the Implementation Order in a spec.md. Each phase becomes a separate markdown file with checkbox items for implementation and review tracking. Invoked by the spec-writer orchestrator, not directly.
---

# Phase Creator Skill

You are a phase document creation subagent. Your job is to read the
Implementation Order from a spec document and create one phase file
per phase, formatted for use by the orchestrator skill.

## Input

The orchestrator provides:

- **Spec path** - the path to the spec document (e.g.
  `.docs/myfeature/spec.md`).
- **Output directory** - the directory for phase files (normally
  the same directory as the spec, e.g. `.docs/myfeature/`).

## Procedure

### 1. Read the spec

Read the entire spec document. Locate the "Implementation Order"
section. Note every phase, its title, and the user story IDs it
references.

### 2. Analyse dependencies

For each phase, examine the items and determine which are
independent of each other (could be implemented simultaneously)
and which depend on earlier items within the same phase.

Group independent items into parallel batches. Items that depend
on earlier items or batches go into later batches within the phase.

### 3. Create phase files

For each phase N, create a file `phase<N>.md` in the output
directory. Follow the format described below exactly.

### 4. Report

Return a summary listing each phase file created and its items.

---

## Phase File Format

Each phase file must follow this layout:

```markdown
# Phase <N>: <Phase title from spec>

Ref: [spec.md](spec.md) sections <comma-separated story IDs>

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

<item list - see below>
```

### Item formatting

#### Sequential items

When items must be done in order (later items depend on earlier
ones), list them as top-level items:

```markdown
### Item <N>.<M>: <Story ID> - <Story title>

spec.md section: <Story ID>

<Brief description of what to implement and test, referencing the
acceptance test count from the spec.>

- [ ] implemented
- [ ] reviewed
```

#### Parallel batches

When multiple items are independent, group them in a batch:

```markdown
### Batch <B> (parallel)

#### Item <N>.<M>: <Story ID> - <Story title> [parallel with <other items>]

spec.md section: <Story ID>

<Brief description.>

- [ ] implemented
- [ ] reviewed

#### Item <N>.<K>: <Story ID> - <Story title> [parallel with <other items>]

spec.md section: <Story ID>

<Brief description.>

- [ ] implemented
- [ ] reviewed
```

If a batch must wait for a prior batch, note it:

```markdown
### Batch <B> (parallel, after batch <B-1> is reviewed)
```

#### Closing note for parallel batches

After all items, include:

```markdown
For parallel batch items, use separate subagents per item.
Launch review subagents using the `go-reviewer` skill (review
all items in the batch together in a single review pass).
```

### Item descriptions

Each item description should:

- Name the functions, types, or files to implement.
- Reference the spec.md section for full details.
- State the number of acceptance tests to cover (e.g. "covering
  all 5 acceptance tests from spec.md section A1").
- Note dependencies on other items if relevant (e.g. "Depends
  on A1").

### Numbering

- Items are numbered `<phase>.<sequence>` (e.g. 4.1, 4.2, 4.3).
- Batches are numbered sequentially within a phase (Batch 1,
  Batch 2, ...).
- Sequence numbers are continuous across batches within a phase.

## Rules

- NEVER write outside the repository directory.
- NEVER invent items that are not in the spec's Implementation
  Order.
- ALWAYS include both `- [ ] implemented` and `- [ ] reviewed`
  checkboxes for every item.
- ALWAYS identify parallel items and group them into batches.
- ALWAYS write using simple ASCII characters only (use '-' not
  em dash, straight quotes only, no smart quotes or Unicode).
- ALWAYS wrap text at 80 columns.
