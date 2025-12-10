# ClickHouse Implementation Specs for Backup Plans

This document specifies how to replace the current custom `treegen`/`ruletree`
database with a ClickHouse‑backed implementation, while keeping the Go
interfaces (`backend`, `ruletree`, `backups`) behaving the same.

The goal is to support ~1.3B files per daily snapshot, hundreds–thousands of
rules (and potentially millions of exact‑path rules), and still provide:

- < 1s latency for:
    - loading the per‑directory “Tree View” (including user/group filters), and
    - adding/updating/removing a rule and seeing updated counts in the UI.
- < 1 hour (target few minutes) to recompute “what needs backing up” for all
    rules against one daily snapshot.

The design below is written so it can be implemented without needing to read
the rest of this codebase.

---

## 1. Behavioural Model and Required Queries

### 1.1. Rules and their semantics

Current behaviour (from `backend/`, `ruletree/`, `backups/`, `frontend/`):

- Rules live in the SQL plan DB (`db.Directories` + `db.Rules`). Each rule:
    - is attached to a *directory path* `dir` (always ending with `/`),
    - has a `Match` pattern relative to that directory (e.g. `*.txt`, `*`),
    - has a `BackupType` (enum: none / ibackup / manual‑* / warn),
    - can be created, updated (same `Match`), or removed.
- A rule with `Match == ""` is normalized to "*" (match everything). A
    `Match` ending in `/` is normalized to `<match>"*"`.
- `validMatch` enforces **no `/`** inside the match, at least one `*`, and no
    NUL bytes, so patterns are filename‑only, not multi‑segment paths.
- Semantics of `*` today (via `wrstat-ui/summary/group`):
    - `*` matches the filename and *all sub‑directories recursively* (i.e. `*`
      effectively behaves like `**`).
    - Requirement: allow future `.gitignore`‑style non‑recursive `*` vs
      recursive `**` and “non‑overridable” rules.

For any given file, exactly one rule is considered to apply:

1. Find all rules whose directory is a prefix of the file path.
2. Keep only those where the file’s leaf name matches `Match` (glob semantics).
3. Take the most specific rule (longest directory prefix). If none, treat as
   “unplanned” (internal rule id `0`).

All aggregate stats in the UI and reports are in terms of this effective rule.

### 1.2. Tree View (`/api/tree` → `frontend/src/data.ts`, `rules.ts`, `summary.ts`, `ruletree.ts`)

For a requested directory `D` (always absolute and ending in `/`):

- Backend returns a JSON `Tree` (see `frontend/src/types.ts`):
    - `RuleSummaries []Rule` for `D` itself:
        - one entry per *rule id* (including id `0` for “unplanned”),
        - each has aggregated `Users []Stats` and `Groups []Stats` where
          `Stats` = { ID (uid/gid), Name, MTime, Files, Size } and represents
          **all files under D’s entire subtree** whose *effective rule id*
          equals this rule id.
    - `Children map[string]*DirSummary` for each immediate child directory
        `D/child/`, with its own `RuleSummaries` over that child’s subtree.
    - `ClaimedBy` (directory owner in plan DB) and `dirDetails` (Frequency,
        ReviewDate, RemoveDate) if the directory is claimed.
    - `Rules map[dirPath]map[ruleID]*Rule` giving the actual rule objects
        (directory + Match + BackupType + Metadata) used by the UI.
    - `Unauthorised []string` listing immediate children that the current user
        is not allowed to see (permission checks combine dir owner and rule stats).
    - `CanClaim` flag, determined from underlying filesystem ownership, telling
        the user if they can claim `D`.
- The frontend derives for each directory:
    - total bytes and file counts per `BackupType`
      (`BackupWarn`/`BackupNone`/`BackupIBackup`/manual), built client‑side from
      per‑rule stats; manual types are grouped together for the summary row.
    - per‑directory rule table (`rules.ts`) showing Match, action, file counts,
      sizes, and edit/remove actions when the user is the claimant.
    - a “rules affecting this subtree” tree view (`ruletree.ts`) which lists
      *every ancestor directory in the subtree that has non‑zero rule stats*,
      with quick navigation back to that directory.
    - user/group filtering (`filter.ts` + `data.ts`) is applied on the client;
      backend must always return per‑user and per‑group stats unfiltered.
- The directory stats and rule summaries are filterable client‑side by user or
    group (see `frontend/src/filter.ts` and `data.ts`). The backend therefore
    must provide *per‑user and per‑group stats*; it must not pre‑apply those
    filters.

Latency requirement: a call to `/api/tree?dir=D` must be < 1s even for large
subtrees (e.g. a project root), assuming ClickHouse is reasonably provisioned.

### 1.3. Rules editing (`/api/rules/*`, `/api/uploadfofn`)

The backend must support:

- Claim / pass / revoke directory claims, with permission checks based on
    underlying filesystem ownership (`RootDir.GetOwner`) and existing rule
    statistics (`isAuthorised`).
- Create / update / remove a rule (single `dir` + `Match`):
    - Validate `Match` string constraints (no `/`, has `*`, no NUL, etc.).
    - Persist to the plan DB.
    - Update the in‑memory rule set and **recompute aggregated rule stats only
        for the affected subtree** (current `ruletree.AddRule`/`RemoveRule`
        behaviour).
    - Ensure `/api/tree` for that directory reflects new stats in < 1s.
- FOFN upload (`/api/uploadfofn`):
    - Accept up to 100 paths or glob patterns.
    - Validate each entry is under the declared parent `dir` and within allowed
        ownership constraints (may require claiming additional sub‑directories).
    - For each pattern, split into a **directory to attach the rule to** and a
        **Match** relative to that directory (see `splitDir`).
    - For existing rules with same `Match` under that directory, update the rule;
        otherwise create a new one.
    - After bulk update, recompute rule stats only for subtrees that changed.

### 1.4. Global report (`/api/report/summary` → `frontend/src/report.ts`)

For a configured list of “report roots” (paths like `/lustre/.../project/`):

- For each root R:
    - Compute a `DirSummary` for R (aggregated `RuleSummaries` for the whole
        subtree under R), ignoring its original `Children`.
    - For each claimed directory `dir` in the plan DB where `dir.Path` is a
        strict prefix descendant of R, compute the `DirSummary` for `dir` itself,
        again ignoring its own children, and attach it as `ds.Children[dir.Path]`.
    - For each such child, also attach `ClaimedBy` and any associated ibackup
        status (via `MultiCache.GetBackupActivity`).
- Collect a global `Rules` map and `Directories` map that together let the UI
    reconstruct, for each claimed directory, which logical rules (and therefore
    `BackupType`s) contributed to its stats.
- The frontend builds a per‑root “traffic light” summary based on recency of
    activity vs recency of backup activity, as well as project owner/BOM
    metadata.

Latency: generating this report can be “a few seconds”, but should not require
full table scans of 1.3B files if avoidable. Pre‑aggregation per snapshot is
acceptable.

### 1.5. Backup generation (`backups.Backup`)

Given a rules DB and a filesystem snapshot, the backup job must:

- For each directory with rules that request automatic ibackup
    (`BackupIBackup`), find *all files* whose effective rule is that rule.
- Emit per‑directory FOFNs and call the ibackup API with:
    - `setName = "plan::" + dir.Path`,
    - `Requestor = dir.ClaimedBy`,
    - `files = [absolute paths]`.

The existing code builds a `group.StateMachine` over rules and walks a tree of
file stats, calling `sm.GetGroup(fi)` for each file and including only
`BackupIBackup` rules.

Performance: a full “what needs to be backed up” run for one daily snapshot
must finish comfortably inside 1 hour (target: a few minutes), even at
1.3B files.

### 1.6. Helper endpoints

ClickHouse must also support:

- `RootDir.IsDirectory(path)` and `/api/getDirectories` → boolean per path.
- `RootDir.GetOwner(path)` used for claim/permission checks.
- `/api/usergroups` (Users/Groups/Owners/BOM) which today is derived from
    `RootDir.Summary("")` over the current snapshot state.

---

## 2. ClickHouse Schema

The ClickHouse schema is snapshot‑oriented **per filesystem root**. Each
`stats.gz` file corresponds to one filesystem mounted at some root path (e.g.
`/lustre/scratch123/humgen/`). For each such root we may have multiple
snapshots over time, and on any given day some roots may have a fresh snapshot
while others are still using an older one; the backend must tolerate mixed
snapshot dates across roots transparently.

The design therefore tracks snapshots *per root* and never assumes that the
entire namespace shares a single global snapshot id or date.

### 2.1. Table: `fs_snapshots`

Tracks snapshots per filesystem root and allows quick switching or
backfilling.

```sql
CREATE TABLE fs_snapshots (
    fs_root       String,   -- e.g. '/lustre/scratch123/humgen/'
    snapshot_id   UInt32,   -- monotonically increasing per fs_root
    snapshot_date Date,     -- logical date of the stats.gz run
    created_at    DateTime DEFAULT now(),
    is_current    UInt8    DEFAULT 0
)
ENGINE = ReplacingMergeTree(is_current)
ORDER BY (fs_root, snapshot_id);
```

For each `fs_root` there should be at most one row with `is_current = 1`.
If a day’s `stats.gz` is missing for a given root, the previous snapshot for
that `fs_root` simply remains current.

### 2.2. Table: `fs_files`

Raw per‑file inventory for each snapshot.

```sql
CREATE TABLE fs_files (
    fs_root     String,
    snapshot_id UInt32,

    -- directory path always ending with '/', absolute (e.g. '/lustre/.../dir/')
    dir_path    String,

    -- file name only, no '/'
    name        String,

    size        UInt64,
    mtime       DateTime64(3),
    uid         UInt32,
    gid         UInt32
)
ENGINE = MergeTree()
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path, name)
SETTINGS index_granularity = 8192;
```

`path` can always be reconstructed as `concat(dir_path, name)`.

### 2.3. Table: `fs_dirs`

Directory metadata, one row per directory path per snapshot. We ingest this
from the directory records in `stats.gz` via the existing WRStat summariser.

```sql
CREATE TABLE fs_dirs (
    fs_root     String,
    snapshot_id UInt32,
    dir_path    String,        -- '/.../dir/'
    uid         UInt32,        -- directory owner uid
    gid         UInt32,        -- directory owner gid
    mtime       DateTime64(3)  -- directory mtime if available, else 0
)
ENGINE = MergeTree()
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path);
```

This table is the source of truth for `IsDirectory` and `GetOwner`.

### 2.4. Table: `fs_dir_stats`

Directory‑level *direct* stats (not recursive) used as a compact base for
tree/rule aggregation. Each row summarises files whose parent is `dir_path`.

```sql
CREATE TABLE fs_dir_stats (
    fs_root     String,
    snapshot_id UInt32,
    dir_path    String,
    uid         UInt32,   -- file uid
    gid         UInt32,   -- file gid
    file_count  UInt64,
    total_size  UInt64,
    max_mtime   DateTime64(3)
)
ENGINE = SummingMergeTree(file_count, total_size, max_mtime)
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path, uid, gid);

CREATE MATERIALIZED VIEW fs_dir_stats_mv TO fs_dir_stats AS
SELECT
    f.fs_root,
    f.snapshot_id,
    f.dir_path,
    f.uid,
    f.gid,
    count()                      AS file_count,
    sum(f.size)                  AS total_size,
    max(f.mtime)                 AS max_mtime
FROM fs_files AS f
GROUP BY
    f.fs_root,
    f.snapshot_id,
    f.dir_path,
    f.uid,
    f.gid;
```

The web UI never needs per‑file detail for counts; it needs recursive directory
stats and rule splits. Recursive aggregation is implemented in Go on top of
this compact base (`fs_dir_stats`), not by scanning `fs_files` per request.

### 2.5. Optional Table: `fs_rule_dir_cache`

For heavy operations (global report, backup planning) we can pre‑materialise
per‑rule recursive directory stats into a cache.

```sql
CREATE TABLE fs_rule_dir_cache (
    fs_root     String,
    snapshot_id UInt32,
    rule_id     UInt64,   -- 0 = unplanned
    dir_path    String,   -- directory at which stats are rooted
    uid         UInt32,   -- file uid
    gid         UInt32,   -- file gid
    file_count  UInt64,
    total_size  UInt64,
    max_mtime   DateTime64(3)
)
ENGINE = SummingMergeTree(file_count, total_size, max_mtime)
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, rule_id, dir_path, uid, gid);
```

This table is populated by an offline Go job that walks the directory
hierarchy and applies the rule state machine (see section 4.2). It is not used
for sub‑second `/api/tree` calls; those use in‑memory structures built from
`fs_dir_stats`.

> Note: the existing small SQLite / MySQL *plan DB* that stores directories
> and rules (`db` package) remains the source of truth for rule definitions and
> directory metadata (Frequency/ReviewDate/RemoveDate/ClaimedBy). ClickHouse
> stores only filesystem stats.

---

## 3. Ingestion from `stats.gz`

We replace `cmd/db.go`’s `treegen.NewTree` pipeline with a ClickHouse
ingestion pipeline, but keep the same source (`stats.gz`). If a daily
`stats.gz` for a root is missing, the previously current snapshot for that root
remains active until the next import.

### 3.1. New Go command: `backup-plans ch-import`

Signature:

```bash
backup-plans ch-import \
    --snapshot-id 20251209 \
    --stats /path/to/stats.gz \
    --ch-dsn clickhouse://user:pass@host:9000/db
```

Responsibilities:

1. Create / register a new `snapshot_id` row in `fs_snapshots`.
2. Stream‑parse `stats.gz` using the existing WRStat parser
     (`stats.NewStatsParser` + `summary.Summariser`).
3. For each directory record:
     - write a row into an in‑memory buffer for `fs_dirs`.
4. For each file record:
     - determine `dir_path` (everything up to and including the last `/`),
     - determine `name` (after last `/`),
     - write a row into `fs_files`.
5. Periodically flush batched inserts into ClickHouse using the native
     protocol (e.g. `github.com/ClickHouse/clickhouse-go/v2`).

`fs_dir_stats` is populated automatically by the materialised view.

### 3.2. Snapshot lifecycle

- Old snapshots can be dropped from ClickHouse by dropping partitions for the
    corresponding `snapshot_id` in `fs_files`, `fs_dirs`, `fs_dir_stats`, and
    `fs_rule_dir_cache`.
- The ingestion job should be designed to complete in < 1 hour for 1.3B files
    (roughly equivalent to the current `treegen` pipeline plus network
    overhead). Separate filesystem roots can be imported in parallel workers.

---

## 4. Rule Evaluation and Aggregation in Go

### 4.1. In‑memory rule automaton

We keep the existing idea of a `group.StateMachine[db.Rule]` (from
`wrstat-ui/summary/group`) for evaluating globs efficiently, but we separate
*filesystem stats* (ClickHouse) from *rule application* (Go).

At backend start‑up (or when rules change):

1. Load all directories and rules from the plan DB (`db.ReadDirectories`,
     `db.ReadRules`).
2. Build an in‑memory `directoryRules map[string]*DirRules` as in
     `ruletree.NewRoot`.
3. Build a `group.StateMachine[db.Rule]` where each state corresponds to a
     byte prefix of a full path `dir_path + Match`. This reproduces the current
     “closest matching rule wins” semantics.

The state machine is reused both for:

- building initial rule aggregates from ClickHouse when the server starts, and
- patching aggregates when rules are created/updated/removed.

### 4.2. Building the rule aggregates once per snapshot

For each **current snapshot per filesystem root** (from
`fs_snapshots.is_current = 1` per `fs_root`), build an in‑memory tree
equivalent to what `treegen` + `ruletree` provide today, but using ClickHouse
as the source. Roots may be on different snapshot dates; aggregates are built
per root and merged into a single namespace keyed by absolute path.

Algorithm sketch (single offline pass, run at startup or after a new
snapshot is marked current):

1. Query ClickHouse for all `fs_dir_stats` rows in the current snapshot for a
    given `fs_root`, ordered by `dir_path`.
2. Walk the directory paths in lexical order, maintaining a stack of
     ancestors.
3. For each directory `D` and each `(uid,gid)` stats row:
     - Conceptually treat this as the *unplanned* rule (`rule_id = 0`) for all
         files directly under `D`.
     - Using the state machine and the set of rules whose directory is a prefix
         of `D`, distribute these counts into per‑rule statistics for that
         directory and for all ancestors that need to see them.
     - Store the results into an in‑memory structure equivalent to
         `ruletree.DirSummary`:
         - keyed by `dir_path`,
         - containing per‑rule aggregates split by user and group.
4. While doing this, also compute global per‑rule, per‑directory aggregates and
    flush them into `fs_rule_dir_cache` for later use by the report and backup
    logic.

This job can be implemented by reusing most of the logic already present in
`ruletree/rulesDir`, but with the “tree” coming from a `dir_path`‑sorted scan
instead of an on‑disk `tree.MemTree`.

Complexity is linear in the number of directories times the number of rules
that intersect each directory; with thousands of rules this is still
practically feasible as an offline step.

### 4.3. `/api/tree` implementation using the in‑memory aggregates

Once the aggregates are built, the `/api/tree` handler becomes a pure
in‑memory operation for the current snapshot set:

1. Look up the requested `dir` in a map `dirSummaries map[string]*DirSummary`
    spanning all current roots.
2. Copy the `DirSummary` for that directory (including `RuleSummaries` and
    immediate `Children` entries) into the HTTP response payload structure
    expected by the frontend (see `backend/tree.go`).
3. Enrich it with:
      - `ClaimedBy` and `dirDetails` from the plan DB / `s.directoryRules`.
      - `Rules` map populated from `s.rules` (as done today).
      - `Unauthorised` list based on `isAuthorised` checks.
      - `CanClaim` derived from `RootDir.GetOwner` (`fs_dirs`) and the current
         user’s uid/gid list.

Because the heavy lifting was done offline, a tree request is now just a few
map lookups and small JSON marshalling and trivially meets the < 1s target.

### 4.4. Patching aggregates when rules change

When a rule is created, updated, or removed via `/api/rules/*` or
`/api/uploadfofn`:

1. Update the plan DB using the existing `db` methods.
2. Under a mutex (`s.rulesMu`), update the in‑memory `directoryRules` and
     `s.rules` maps.
3. Rebuild the state machine (cheap compared to snapshots).
4. Recompute aggregates only for the affected subtree:
         - Find the smallest directory `D` whose subtree covers all files that could
             change effective rule as a result of this modification. For a simple rule
             on `dir`, that is `dir` itself; for a FOFN upload it may span a handful of
             sibling directories.
         - For `D` and all descendants within the same `fs_root`, recompute
             per‑rule aggregates by replaying the `fs_dir_stats` rows for those
             directories through the new state machine.
         - Update both the in‑memory `dirSummaries` and the corresponding rows in
             `fs_rule_dir_cache`.

Because `fs_dir_stats` is compact (one row per directory/uid/gid combination,
not per file), and because rule changes are localised, this patching step is
fast enough for interactive rule edits.

> This preserves the current behaviour where adding a rule immediately updates
> all counts in the UI without re‑running the full daily import.

### 4.5. `.gitignore`‑style `*` vs `**` and non‑overridable rules

The existing system treats `*` as “match this directory and all subdirs
recursively”. To support `*` vs `**` and non‑overridable rules:

1. Extend the plan DB’s `rules` table with two extra columns:
     - `recursive` (UInt8): 0 = match only this directory, 1 = recurse into
         subdirs (`**` semantics).
     - `priority` (UInt8): 0 = normal rule (can be overridden by deeper rules),
         1 = non‑overridable.
2. At the API level:
     - Keep existing `Match` semantics for now.
     - When introducing `**` in the UI, set `recursive = 1`; for plain `*` set
         `recursive = 0` and adjust the state machine so that it does not extend
         the match to children unless `recursive = 1`.
     - For non‑overridable rules, teach the state machine to ignore any lower
         priority rules that would otherwise override them in deeper directories.
3. The ClickHouse schema does not need to change; only the rule evaluation
     logic in Go (state machine) must be extended to consider these flags when
     picking the effective rule for a file.

---

## 5. Mapping Existing Code to the New Design

This section shows, at a high level, how the main packages would use
ClickHouse.

### 5.1. `ruletree.RootDir`

- Replace the direct use of `tree.MemTree` in `ruletree` with a new
    `ClickhouseRoot` implementation that:
    - on construction, loads aggregates from ClickHouse for the current
        snapshot and builds `dirSummaries` and `directoryRules` as described in
        section 4.2;
    - exposes the same methods as today: `Summary(path)`, `GetOwner(path)`,
        `IsDirectory(path)`, `AddTree` (no‑op; snapshots now live in ClickHouse),
        `AddRule`, and `RemoveRule`.
- `backend.Server` continues to call `ruletree.NewRoot` with the initial
    rules; the implementation of `NewRoot` is changed to create a
    `ClickhouseRoot` rather than a `tree.MemTree`‑based root.

### 5.2. `/api/tree` and frontend

No API shape changes are required. The only change is that `Summary` and
`GetOwner` now obtain their data from ClickHouse‑backed aggregates instead of
an on‑disk `.db` file.

### 5.3. Global report (`/api/report/summary`)

The report implementation can either:

- Use the in‑memory `dirSummaries` and `fs_rule_dir_cache` to compute, for
    each report root and each claimed directory under it, the per‑rule per‑user
    stats exactly as today, or
- For a more stateless approach, query `fs_rule_dir_cache` directly for the
    relevant `rule_id`/`dir_path` ranges and aggregate in Go.

In both cases, ClickHouse is only serving pre‑aggregated stats; no per‑file
scans are needed when rendering the report.

### 5.4. Backup generation

For the backup job there are two reasonable implementations:

1. **Direct ClickHouse scan (stateless)**

     - For each rule `R` that has `BackupType = BackupIBackup`:
         - Build a `WHERE` clause that encodes `R`’s directory prefix and `Match`
             pattern (`LIKE`/`match` with proper escaping).
         - Encode overriding semantics by excluding paths that would be matched by
             more specific rules with non‑`BackupIBackup` types. This can be done by
             computing a list of “blocked” sub‑prefixes for `R` and adding
             `AND NOT (dir_path LIKE blocked_prefix || '%')` clauses.
         - Run a ClickHouse query:

             ```sql
             SELECT concat(dir_path, name) AS path
             FROM   fs_files
             WHERE  snapshot_id = {current_snapshot_id}
                 AND  dir_path LIKE {R.dir_path_prefix}
                 AND  name      LIKE {pattern_from_Match}
                 AND  {not_overridden_conditions}
             ORDER BY dir_path, name
             ```

     - Stream rows directly into ibackup FOFNs.

     This approach keeps the backup job stateless but may scan a large fraction
     of `fs_files` for very broad rules.

2. **Reuse the in‑memory aggregates (stateful)**

     - Extend the offline aggregation (section 4.2) to also record, for each
         rule and directory, the list of *directories with any matching files*.
     - At backup time, for each `BackupIBackup` rule and each directory where it
         has non‑zero stats, query ClickHouse only for that directory’s files and
         re‑apply the rule’s pattern to produce the actual file paths.

In both variants, the constraints (< 1 hour, few minutes preferred) are
achievable because rules are relatively few compared to files, and queries can
be batched by directory prefix.

---

## 6. Implementation Checklist

1. **Create ClickHouse schema** as in section 2.
2. **Implement `ch-import`** command that reads `stats.gz` and fills
     `fs_files` and `fs_dirs`.
3. **Write an aggregation job** that builds in‑memory `dirSummaries` and
     optionally populates `fs_rule_dir_cache` from ClickHouse and the plan DB
     rules.
4. **Rewrite `ruletree.NewRoot` / `RootDir`** to use ClickHouse‑backed
     aggregates instead of `tree.MemTree`.
5. **Adapt `backend`**:
     - `Tree`, `Summary`, `Fofn`, `GetDirectories`, `UserGroups` continue to use
         `RootDir` and the plan DB as now; no API wire change is needed.
6. **Adapt `backups.Backup`** to either use ClickHouse directly or the
     aggregated in‑memory structures as described in 5.4.
7. **Add tests and small fixtures** that:
     - import a tiny `stats.gz` into ClickHouse,
     - define a small ruleset in the plan DB,
     - check that `/api/tree`, `/api/report/summary`, and the backup job all
         behave identically (within rounding) to the current `treegen`‑based
         implementation.

This plan keeps ClickHouse as the single source of filesystem stats, preserves
existing rule semantics (with room to add `*` vs `**` and non‑overridable
rules), and keeps interactive behaviour fast by pushing heavy aggregation work
into offline jobs and compact in‑memory structures.
