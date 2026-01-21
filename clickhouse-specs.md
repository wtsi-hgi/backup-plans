# ClickHouse Implementation Specs for Backup Plans

This document specifies how to replace the custom `treegen`/`ruletree`
database with ClickHouse, while keeping Go interfaces behaving the same.

## Executive Summary

| Metric | Requirement |
|--------|-------------|
| Total files per snapshot | ~1.3 billion |
| Glob-pattern rules | Hundreds to thousands, < 1s evaluation |
| Filesystems (roots) | Multiple (5-10), independent snapshot dates |
| `/api/tree` latency | < 1s |
| Rule add/update with UI refresh | < 1s |
| Full backup computation | < 1hr, target few minutes |
| Database refresh (per root) | < 1hr per root |

### Key Design Decisions

1. **ClickHouse stores filesystem stats only** - plan DB (SQLite/MySQL) remains source of truth for rules and directory claims
2. **Per-root snapshots** - each filesystem root has independent snapshots; the system operates on the latest available snapshot for each root (mixed dates supported)
3. **In-memory aggregates for interactive queries** - `/api/tree` uses pre-computed in-memory structures
4. **State machine for rule matching** - reuse `wrstat-ui/summary/group` automaton for O(path-length) rule evaluation
5. **Incremental updates** - rule changes trigger partial re-aggregation

---

## 1. Rules Semantics

### 1.1. Rule Storage (Plan DB)

Rules remain in SQLite/MySQL (`db` package):

```sql
CREATE TABLE directories (
    id INTEGER PRIMARY KEY, path TEXT UNIQUE, claimed_by TEXT,
    frequency INTEGER, review_date INTEGER, remove_date INTEGER,
    created INTEGER, modified INTEGER
);

CREATE TABLE rules (
    id INTEGER PRIMARY KEY, directory_id INTEGER REFERENCES directories(id),
    backup_type INTEGER, metadata TEXT, match_str TEXT,
    created INTEGER, modified INTEGER
);
```

**BackupType:** 0=None, 1=IBackup, 2=ManualIBackup, 3=ManualGit, 4=ManualUnchecked, 5=ManualPrefect

### 1.2. Match Constraints

- UI-created rules (`CreateRule`/`UpdateRule`) must contain `*` (wildcard)
- UI rules must NOT contain `/` in `match` (filename-only patterns; the directory path is stored separately)
- FOFN-imported rules can be exact filenames (no `*`) for individual file paths. The FOFN
  handler uses `splitDir()` to extract the directory portion up to the last `/` before any `*`,
  creating/claiming intermediate directories as needed. So `/data/project/subdir/file.txt` becomes
  `directory.path = "/data/project/subdir/"` and `rule.match = "file.txt"`.
- In all cases `match` is interpreted relative to the directory; the effective pattern is `dir.Path + rule.Match`
- `*` currently matches recursively into subdirectories (via `group.StateMachine`)

### 1.3. Effective Rule Determination

For file at path `P`:
1. Find rules whose `directory.path` is a prefix of file's parent
2. Filter by pattern match on basename
3. Select rule with longest `directory.path` ("most specific wins")
4. Default to "unplanned" (rule ID = 0) if no match

**Example:**
```
Rules:
  /data/project/*        -> BackupIBackup  (rule 1)
  /data/project/temp-*   -> BackupNone     (rule 2)
  /data/project/archive/*.gz -> BackupIBackup (rule 3)

Files:
  /data/project/file.txt        -> rule 1 (matches *)
  /data/project/temp-cache.dat  -> rule 2 (matches temp-*)
  /data/project/archive/data.gz -> rule 3 (matches *.gz)
  /other/path/file.txt          -> rule 0 (unplanned)
```

### 1.4. Advanced Rule Semantics (`*` vs `**` and Non-Overridable Rules)

To support `.gitignore`-style semantics and non-overridable rules, the rule schema includes:

```sql
ALTER TABLE rules ADD COLUMN recursive TINYINT NOT NULL DEFAULT 1;
ALTER TABLE rules ADD COLUMN no_override TINYINT NOT NULL DEFAULT 0;
```

- `recursive = 0`: `*` matches only files in the immediate directory (non-recursive, like shell glob)
- `recursive = 1`: `*` matches recursively into subdirectories (current behavior, `**` equivalent)
- `no_override = 0`: Normal rule (can be overridden by more-specific child rules)
- `no_override = 1`: Non-overridable rule; implemented by automatically creating
  identical rules for all subdirectories during aggregation

**Migration:** Existing rules get `recursive = 1, no_override = 0` to preserve behavior.

**State Machine Modifications:**

The current `group.StateMachine` from `wrstat-ui` treats `*` as matching `/` (recursive).
To support non-recursive matching:

1. For rules with `recursive = 0`, replace `*` with a pattern that matches any character
   except `/` before adding to the state machine. This makes `*` behave like shell globbing.
   
2. For rules with `recursive = 1` (default, current behavior), keep `*` matching `/`.

**Non-Overridable Rules Implementation:**

When building the state machine, for each rule with `no_override = 1`:
1. Query ClickHouse for all subdirectories under the rule's directory path
2. Add a synthetic rule entry for each subdirectory with the same match pattern
3. These synthetic entries ensure the rule "wins" at every level since it becomes
   the most-specific match at each directory depth

This approach requires no changes to the state machine logic itself—the existing
"most specific directory wins" semantics naturally handle it.

---

## 2. ClickHouse Schema

### 2.1. Table: `fs_snapshots`

Tracks available snapshots per filesystem root. Uses `ReplacingMergeTree` to handle
updates (e.g., toggling `is_current`). At most one row per `fs_root` should have
`is_current = 1`. Query with `FINAL` to get deduplicated results.

```sql
CREATE TABLE fs_snapshots (
    fs_root String, snapshot_id UInt32, snapshot_date Date,
    file_count UInt64 DEFAULT 0, total_size UInt64 DEFAULT 0,
    created_at DateTime DEFAULT now(), is_current UInt8 DEFAULT 0
) ENGINE = ReplacingMergeTree(created_at)
ORDER BY (fs_root, snapshot_id);
```

### 2.2. Table: `fs_files`

The main file inventory table. Each row represents one file. Partitioned by
`(fs_root, snapshot_id)` to enable instant deletion of old snapshots via
`DROP PARTITION`. Ordered by path for efficient prefix scans.

```sql
CREATE TABLE fs_files (
    fs_root String, snapshot_id UInt32,
    dir_path String, name String,  -- dir_path ends with '/'
    size UInt64, mtime DateTime64(3), uid UInt32, gid UInt32
) ENGINE = MergeTree()
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path, name);
```

**Path conventions:**

- `fs_root` is the logical filesystem root as seen by the UI, e.g. `/lustre/scratch/`
- `dir_path` is the directory path containing the file, always ending with `/`, e.g. `/lustre/scratch/project1/foo/`
- The full absolute path of a file is `dir_path || name`
- Plan DB directory paths (`directories.path`) use the same absolute-path format so that `dir.Path + rule.Match` aligns with the ClickHouse paths

**Note:** Using absolute paths for `dir_path` (rather than relative) simplifies rule matching since
rules in plan DB use absolute paths like `/lustre/scratch/project/`.

**Storage estimate for 1.3B files:**
- Average row: ~150 bytes (paths dominate)
- With LZ4: ~40-60 GB
- With ZSTD: ~25-40 GB

### 2.3. Table: `fs_dirs`

Directory metadata for `IsDirectory()` checks and `GetOwner()` lookups.
Used during claim validation to verify the user owns the directory.

```sql
CREATE TABLE fs_dirs (
    fs_root String, snapshot_id UInt32,
    dir_path String, uid UInt32, gid UInt32, mtime DateTime64(3)
) ENGINE = MergeTree()
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path);
```

**Storage estimate:** ~5-10 million directories -> ~1-2 GB compressed

### 2.4. Table: `fs_dir_stats`

Pre-aggregated per-directory statistics, grouped by file UID/GID. This reduces
the data volume from 1.3B file rows to ~50-100M aggregated rows (one per
directory/uid/gid combination). The materialized view auto-populates this
table during ingestion.

`SummingMergeTree` automatically merges rows with the same key by summing
`file_count` and `total_size` columns.

```sql
CREATE TABLE fs_dir_stats (
    fs_root String, snapshot_id UInt32, dir_path String,
    uid UInt32, gid UInt32, file_count UInt64, total_size UInt64, max_mtime DateTime64(3)
) ENGINE = SummingMergeTree((file_count, total_size))
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path, uid, gid);

CREATE MATERIALIZED VIEW fs_dir_stats_mv TO fs_dir_stats AS
SELECT fs_root, snapshot_id, dir_path, uid, gid,
       count() AS file_count, sum(size) AS total_size, max(mtime) AS max_mtime
FROM fs_files GROUP BY fs_root, snapshot_id, dir_path, uid, gid;
```

**Usage:** Optional helper for quickly computing per-directory totals (for
coarse reporting or sanity checks). Rule-aware aggregation and ownership checks
still rely on `fs_files` (per-file paths) and `fs_dirs` (directory metadata);
`fs_dir_stats` is *not* sufficient on its own for assigning files to rules.

### 2.5. Snapshot Lifecycle

**Add snapshot:** Import to `fs_files`/`fs_dirs`, mark current via INSERT to `fs_snapshots`.

**Delete snapshot:** `ALTER TABLE ... DROP PARTITION (fs_root, snapshot_id)` - instantaneous.

**Mixed dates:** Each `fs_root` tracks its own `is_current` snapshot independently.
When a stats.gz file for a particular filesystem is not generated on a given day,
the system continues using the most recent snapshot for that root while other
roots may have newer snapshots. The web server and backup command must:
1. Query `fs_snapshots FINAL WHERE is_current = 1` to get the active snapshot per root
2. Join or filter queries by `(fs_root, snapshot_id)` pairs from step 1

### 2.6. Mapping Paths to Roots

Given an absolute path, determine which `fs_root` it belongs to by finding
the longest matching prefix from `currentSnapshots`:

```go
func (c *CHRootDir) findRoot(absPath string) (fsRoot string, snapshotID uint32, ok bool) {
    bestLen := 0
    for root, snapID := range c.currentSnapshots {
        if strings.HasPrefix(absPath, root) && len(root) > bestLen {
            fsRoot, snapshotID, bestLen = root, snapID, len(root)
        }
    }
    return fsRoot, snapshotID, bestLen > 0
}
```

This is needed because claimed directories/rules use absolute paths like
`/lustre/scratch123/project/` and we need to know which `(fs_root, snapshot_id)`
to query in ClickHouse.

---

## 3. Ingestion

### 3.1. Command: `backup-plans ch-import`

```bash
backup-plans ch-import --stats /path/to/stats.gz --ch-dsn "clickhouse://..." [--mark-current]
```

### 3.2. Algorithm

1. Open `stats.gz`, parse with `summary.NewSummariser`
2. Generate `snapshot_id` as Unix timestamp (seconds) or date-based integer (YYYYMMDD)
3. Batch insert to `fs_files` and `fs_dirs` (flush every 100k rows)
4. Register snapshot in `fs_snapshots`
5. If `--mark-current`, set `is_current = 1`

**Snapshot ID generation:** Use a consistent format like `YYYYMMDD` (e.g., 20251210)
or Unix timestamp. The ID must be unique per `(fs_root, snapshot_id)` pair.

**Determining `fs_root`:** The stats.gz file contains paths rooted at the
filesystem mount point (e.g., `/lustre/scratch123/`). The `fs_root` is extracted
from the first directory path encountered. In the current `cmd/db.go`, the
`summary.NewSummariser` processes directories depth-first; the root is the
first `DirectoryPath` with `Parent == nil`. The `ch-import` command should
accept `--fs-root` as an explicit override, falling back to auto-detection.

**Performance Target:** < 45 min for 1.3B files total.
**Parallelism:** Since `stats.gz` files are per-filesystem, multiple `ch-import` processes can run in parallel for different roots to maximize throughput.

---

## 4. Rule Evaluation and Aggregation

### 4.1. In-Memory Structures

The backend Server maintains these structures for fast `/api/tree` responses:

- **`directoryRules map[string]*DirRules`** - Claimed directories and their rules,
  loaded from plan DB. Each entry contains the directory's claim info (claimedBy,
  frequency, reviewDate, removeDate) plus a map of rules keyed by match pattern.

- **`stateMachine group.StateMachine[db.Rule]`** - Byte-by-byte automaton from
  `wrstat-ui/summary/group`. Evaluates file paths in O(path-length) time.
  Handles `*` wildcards and implements "most specific wins" semantics.
  
  **State Machine API:**
  - `group.NewStatemachine(rules []PathGroup[T])` - Creates a new state machine
  - `sm.GetGroup(fi *summary.FileInfo) *T` - Returns the matching rule for a file,
    or `nil` if no rule matches. The `FileInfo.Path` (a `*DirectoryPath` linked list)
    and `FileInfo.Name` are used to construct the full path for matching.
  - `sm.GetStateString(pathSegment string) State[T]` - Advances the state machine
    by a path segment (used for incremental traversal, not needed for ClickHouse impl)

- **`dirSummaries map[string]*DirSummary`** - Recursive stats per directory,
  built by streaming `fs_files` and applying the state machine. Each summary
    contains per-rule aggregates broken down by user/group. The structure is
    intentionally compatible with the existing `ruletree.DirSummary` so that
    `/api/tree`, `/api/report/summary`, and `/api/usergroups` can keep their
    JSON shapes unchanged.

- **`exactRules map[string]*db.Rule`** - Hash map of full file paths to rules
  for rules without `*` in their match (exact path matches from FOFN imports).

**Query routing decision table:**

| Operation | Data Source | Notes |
|-----------|-------------|-------|
| `/api/tree` stats | In-memory `dirSummaries` | Pre-computed, <10ms |
| `/api/tree` children list | In-memory + ClickHouse fallback | Query `fs_dirs` for uncached dirs |
| `GetOwner(path)` | In-memory `dirSummaries` or ClickHouse | Cache hit first, then `fs_dirs` |
| `IsDirectory(path)` | In-memory or ClickHouse | Check cache, then query `fs_dirs` |
| Rule create/update | ClickHouse re-query of subtree | Stream `fs_files`, update in-memory |
| Backup job | ClickHouse streaming | No in-memory cache available |

### 4.2. Building Aggregates

At startup or when a new snapshot becomes current:

1. **Load current snapshots** - Query `fs_snapshots FINAL WHERE is_current = 1` to get
   active `(fs_root, snapshot_id)` pairs. Store in `currentSnapshots map[string]uint32`.

2. **Load claimed directories and rules from plan DB** - Populate `directoryRules`,
   `dirs`, and `rules` maps exactly as `backend.Server.loadRules()` does today.

3. **Rebuild state machine** from plan DB rules. Each rule becomes a
   `PathGroup{Path: dir.Path + rule.Match, Group: rule}`. The state machine
   compiles these into an automaton. Also populate `exactRules map[string]*db.Rule`
   for rules without `*` in their match pattern.

4. **Single-pass aggregation over files (only where rules exist)**:
    First build the minimal set of directory prefixes that need summaries,
    exactly as `ruletree` does today. Aggregation must read filenames because
    rule matching is filename-sensitive; `fs_dir_stats` alone is insufficient
    except for pure catch-all (`*`) rules.

     - Start from all directories in `directoryRules` (i.e. those that have at
         least one rule).
     - For each such directory, add it and all ancestors up to the filesystem
         root to a `ruleDirPrefixes map[string]bool`.
     - Then compress this set so that no prefix is a descendant of another
         (i.e. keep only minimal prefixes, so `/lustre/project/` implies we can
         drop `/lustre/project/subdir/` from the scan set).

     For each `(fs_root, snapshot_id)` in `currentSnapshots`, and for each
     prefix `p` in this minimal set that lies under that root, stream only the
     `fs_files` rows whose `dir_path` starts with `p`, ordered by
     `(dir_path, name)`. For each file row
     `(fs_root, snapshot_id, dir_path, name, size, mtime, uid, gid)`:

     a. Build the full path `fullPath = dir_path || name`.

     b. If `exactRules[fullPath]` exists, use that rule. Otherwise construct a
            `summary.FileInfo` with `Path` (a `DirectoryPath` chain built from
            `dir_path`) and `Name` (the filename), then call `stateMachine.GetGroup(fi)`
            to get the winning rule (or `nil` for "unplanned"). This preserves the
            existing "most specific directory path wins" semantics.
            
            ```go
            // Helper to build FileInfo from ClickHouse row
            func buildFileInfo(dirPath, name string) *summary.FileInfo {
                // Parse dirPath into DirectoryPath linked list
                dp := parseDirectoryPath(dirPath)  // e.g., "/a/b/c/" -> chain of DirectoryPath
                return &summary.FileInfo{
                    Path: dp,
                    Name: []byte(name),
                }
            }
            ```

     c. Determine the directory to attribute stats to (the file's parent,
            i.e. `dir_path`) and look up/create its `DirSummary`.

     d. Update `RuleSummaries[ruleID].Users[uid]` and
            `RuleSummaries[ruleID].Groups[gid]` by increasing `Files`/`Size` and
            taking the max of `MTime`. For files with no matching rule, use
            `ruleID = 0` (the "unplanned"/`BackupWarn` bucket).

     This means:

     - Files in parts of the filesystem where there are no rules anywhere in
         the subtree are never scanned (they are invisible to the UI today as
         well).
     - All files under any directory that has rules (or whose descendants have
         rules) are still scanned.

     **Optimization for Recursive Rules:**
     If a directory `D` has a recursive rule (`*`) and there are **no other rules** in its entire subtree (checked via the rule prefix set), we can skip streaming `fs_files` for this subtree. Instead, query `fs_dir_stats` to get pre-aggregated sums for `D` and all its descendants.
     ```sql
     SELECT dir_path, uid, gid, sum(file_count), sum(total_size), max(max_mtime)
     FROM fs_dir_stats
     WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
     GROUP BY dir_path, uid, gid
     ```
     This dramatically speeds up aggregation for large subtrees covered by a single policy.

     Implementations may choose to buffer per-directory aggregates in memory
     while streaming, or to shard the work by `(fs_root, snapshot_id)` and/or
     by prefix to exploit parallelism. The key requirement is that each file is
     assigned to exactly one effective rule, as it is today, and that only
     rule-relevant subtrees are read from ClickHouse.

5. **Aggregate upward**: After all files have been processed, walk directories
    deepest-first (reverse `dir_path` order). For each directory, add its
    per-rule stats into all ancestors, mirroring the current `ruletree`
    behaviour where a rule that matches files in a subtree contributes to all
    parent directories.

**Performance:** Linear in the number of files under rule-covered subtrees.
In practice this should be dominated by ClickHouse scan speed; with streamed
reads of `fs_files` and multi-core aggregation in Go, the goal is to stay
within the existing rebuild budget (tens of minutes for ~1.3B files when all
files are under rules, much faster when rules cover only a subset).

**Memory Consumption:** The `dirSummaries` map must hold summaries for all
directories that have files or have rules in their subtree. Estimate:
- Per-directory overhead: ~200 bytes + (num_rules × num_users × 50 bytes)
- With 10M directories and sparse rules: ~2-4 GB
- If rules are very broad (many users per rule), may need more
- **Mitigation:** For directories deeper than a configurable threshold with no
  rules in their subtree, don't store individual `DirSummary` entries; their
  stats are aggregated into the nearest ancestor with rules.

### 4.3. `/api/tree` Implementation

Pure memory lookup + JSON serialization from `dirSummaries` and
`directoryRules` (plan DB). **Latency:** < 10ms. The frontend applies
user/group filters client-side, so the handler must always return full
per-rule per-user/per-group stats.

**Child listing:** The `Children` map in `DirSummary` must list every immediate
child directory, even when that child currently contributes zero rule stats.
During the initial aggregation phase (section 4.2), also query `fs_dirs` to
populate the complete child list with ownership (uid/gid) for each directory.
This enables users to navigate and claim previously-unruled directories.

**Implementation detail:** For each directory in `dirSummaries`, ensure its
`Children` map includes all immediate subdirectories from `fs_dirs`. For
children that have no matching files (zero stats), create a `DirSummary` with
empty `RuleSummaries` but populated `uid/gid` from `fs_dirs`. The `/api/tree`
handler only returns immediate children (one level), not the full tree.

**Query to populate children:**
```sql
SELECT dir_path, uid, gid FROM fs_dirs
WHERE fs_root = ? AND snapshot_id = ?
  AND dir_path LIKE ? || '%/'
  AND dir_path NOT LIKE ? || '%/%/'
ORDER BY dir_path;
```
(This selects immediate children of a given directory path.)

**Lazy child loading:** The `/api/tree?dir=X` endpoint returns `Children` as
a shallow map containing only immediate children of `X` (one level deep). The
`Children` entries themselves have empty `Children` maps. The frontend navigates
by making subsequent `/api/tree` calls for deeper directories. This means:
- The in-memory `dirSummaries` structure only needs immediate children populated
  for directories the user has actually navigated to
- On initial aggregate build, populate `Children` lazily: only add child entries
  when a file is processed in that child directory, or when `/api/tree` is called
- For unclaimed/unruled directories, query `fs_dirs` on-demand when `/api/tree`
  is called for that path

The handler must continue to return the existing `Tree` JSON structure
(`frontend/src/types.ts`):

```typescript
type Tree = {
  RuleSummaries: { ID: number, Users: Stats[], Groups: Stats[] }[];
  Children: Record<string, { RuleSummaries: ..., Children: ... }>;
  ClaimedBy: string;
  Rules: Record<string, Record<number, Rule>>;  // dir path -> rule ID -> Rule
  Unauthorised: string[];   // child names user cannot view
  CanClaim: boolean;
  Frequency: number;
  ReviewDate: number;
  RemoveDate: number;
};
```

**Authorization logic** (same as current implementation in `backend/tree.go`):
- User is authorized to view a directory if ANY of:
  - `uid == dir.uid` (user owns the directory)
  - `dir.gid in user.groups` (user is in directory's group)
  - `user.groups contains adminGroup` (user is admin)
  - User's UID appears in any `RuleSummary.Users` for this directory (user owns files in subtree)
  - Any of user's GIDs appear in any `RuleSummary.Groups` for this directory (user's group owns files)
- `CanClaim` is true only if user owns the directory OR is in directory's group (not just file ownership)
- `Unauthorised` lists child directory names where user lacks authorization (used to show lock icons)

### 4.4. Incremental Updates

On rule create/update/remove:

1. **Persist to plan DB** - Insert/update/delete the rule row.

2. **Rebuild state machine** (~100ms for thousands of rules) - The entire
   automaton is rebuilt since rule changes can affect matching.
   Also rebuild `exactRules` map.

3. **Re-aggregate the affected subtree** - Because the "most specific wins"
   semantics means adding/removing a rule can change what other rules match for
   files in the subtree, we must re-evaluate all files in the affected area:
   - Identify the minimum subtree: the rule's directory path and all descendants
   - Clear existing aggregates for directories in this subtree
   - Re-stream files from ClickHouse for this subtree and re-evaluate each
     against the rebuilt state machine
   - **Optimization for pure catch-all**: If the subtree has only one rule (`*`)
     and no child rules exist, query `fs_dir_stats` instead of `fs_files`
   - **Optimization for filename-sensitive**: Use ClickHouse `match(name, pattern)`
     to pre-filter files in the query, reducing data transfer:
     ```sql
     SELECT dir_path, name, uid, gid, size, mtime
     FROM fs_files
     WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
     ORDER BY dir_path, name
     ```

4. **Propagate to ancestors** - After subtree re-aggregation, walk up to the
   root and recompute ancestor totals by summing their children's stats.

**Latency:** < 1s for typical updates (subtrees with < 1M files).
For very large subtrees, may take 2-5s; consider async update with UI polling.

### 4.5. Glob-to-Regex Conversion

ClickHouse's `match(name, pattern)` uses RE2 regex syntax. Convert glob patterns:

```go
func globToRegex(glob string, recursive bool) string {
    var b strings.Builder
    b.WriteString("^")
    for _, c := range glob {
        switch c {
        case '*':
            if recursive {
                b.WriteString(".*")  // matches any char including /
            } else {
                b.WriteString("[^/]*")  // matches any char except /
            }
        case '?':
            b.WriteString(".")
        case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
            b.WriteString("\\")
            b.WriteRune(c)
        default:
            b.WriteRune(c)
        }
    }
    b.WriteString("$")
    return b.String()
}
```

For exact-path rules (no `*`), skip regex matching and use exact string comparison.

### 4.6. `/api/report/summary` Implementation

The report summary endpoint returns aggregated stats for all configured report roots:

```typescript
type ReportSummary = {
  Summaries: Record<string, ClaimedDir>;  // root path -> summary
  Rules: Record<number, Rule>;            // rule ID -> rule details
  Directories: Record<string, number[]>;  // dir path -> rule IDs in that dir
  BackupStatus: Record<string, BackupStatus>;  // dir path -> ibackup status
};
```

Implementation:
1. For each report root, retrieve `DirSummary` from `dirSummaries`
2. For each claimed directory under the root, include as child in `ClaimedDir.Children`
3. Collect all referenced rules into the `Rules` map
4. Query ibackup cache for backup status per claimed directory

### 4.7. Exact-Path Rules at Scale

To support the possibility of **millions of explicit full-path rules** (from FOFN imports)
while keeping the automaton small, split rule handling into:

- **Glob rules** (with `*` in `match`): compiled into the `stateMachine`
- **Exact rules** (no `*` in `match`): kept in an in-memory hash
    `exactRules map[string]*db.Rule` keyed by full path (`dir.Path + match`)

When assigning a rule to a file during aggregation or backup:

1. Check `exactRules[fullPath]` first
2. If no exact rule matches, fall back to `stateMachine.GetGroup(fileInfo)`

This keeps state-machine size proportional only to the number of glob-style
rules (thousands), while still allowing millions of explicit file-path rules.

**Note on exact-path rule precedence:** An exact-path rule is inherently the
"most specific" match for that file. If both an exact rule and a glob rule
could match, the exact rule wins (checked first). This matches the "most
specific directory wins" semantics since the exact rule's directory is the
file's immediate parent.

### 4.8. `/api/usergroups` Implementation

Returns all unique users and groups that have files in the filesystem:

```typescript
type UserGroups = {
  Users: string[];   // usernames from RuleSummaries across all roots
  Groups: string[];  // group names from RuleSummaries across all roots
  Owners: Record<string, string[]>;  // owner -> group names (from CSV)
  BOM: Record<string, string[]>;     // BOM name -> group names (from CSV)
};
```

Built by iterating root `DirSummary.RuleSummaries` and collecting unique names.

### 4.9. `/api/getDirectories` Implementation

The frontend calls this endpoint with a list of paths to check if they exist
as directories (used during FOFN validation):

```go
// POST /api/getDirectories - body is JSON array of paths
// Returns: JSON array of booleans (same order as input)
func (s *Server) GetDirectories(paths []string) []bool {
    results := make([]bool, len(paths))
    for i, path := range paths {
        results[i] = s.rootDir.IsDirectory(path)
    }
    return results
}
```

The `IsDirectory` method queries `fs_dirs` to check if the path exists.

---

## 5. Backup Job

The `backup` command runs statelessly on a separate machine with no shared
in-memory cache from the web server. It must rebuild all necessary state from
plan DB and ClickHouse on each run.

### 5.1. Architecture

```
backup cmd
    ├── connects to plan DB (rules + directories)
    ├── connects to ClickHouse (file data)
    ├── builds state machine from rules
    ├── streams files from ClickHouse per claimed directory
    ├── evaluates each file against state machine
    └── submits matching BackupIBackup files to ibackup
```

### 5.2. Implementation

```go
func Backup(ctx context.Context, planDB *db.DBRO, chConn clickhouse.Conn,
    client *ibackup.MultiClient) ([]SetInfo, error) {
    
    // 1. Load current snapshots for all roots
    snapshots := loadCurrentSnapshots(chConn)  // map[fs_root]snapshot_id
    
    // 2. Load directories and build rule structures
    dirs := loadDirectories(planDB)           // map[dirID]*Directory
    sm := buildStateMachine(planDB, dirs)     // state machine for glob rules
    exactRules := buildExactRuleMap(planDB, dirs)  // map[fullPath]*Rule
    
    // 3. Collect files to backup, grouped by directory (for ibackup sets)
    setFofns := make(map[int64][]string)  // dirID -> list of file paths
    
    // 4. Build minimal prefix set for directories with BackupIBackup rules
    prefixes := buildMinimalPrefixes(dirs, planDB)  // only dirs that can lead to ibackup
    
    // 5. Stream files under those prefixes
    for _, prefix := range prefixes {
        fsRoot, snapshotID := findSnapshot(snapshots, prefix)
        streamFiles(ctx, chConn, fsRoot, snapshotID, prefix, func(dirPath, name string) {
            fullPath := dirPath + name
            
            // Determine effective rule (same logic as web server aggregation)
            var rule *db.Rule
            if r, ok := exactRules[fullPath]; ok {
                rule = r
            } else {
                rule = sm.GetGroupForPath(fullPath)  // may return nil
            }
            
            // Only backup if rule is BackupIBackup
            if rule != nil && rule.BackupType == db.BackupIBackup {
                setFofns[rule.DirID()] = append(setFofns[rule.DirID()], fullPath)
            }
        })
    }
    
    return submitToIBackup(client, setFofns, dirs)
}
```

**Why re-evaluate rules during streaming:** A file under `/data/project/` might match
rule A at `/data/project/*`, but if rule B at `/data/project/subdir/*.txt` is more
specific for that file, rule B wins. The backup command cannot simply trust that
a directory's rule applies to all descendant files; it must apply the state machine
to each file path, just like the aggregation phase does.
```

### 5.3. Performance & Optimization

| Scale | Directories | Files | Time (Naive) | Time (Optimized) |
|-------|-------------|-------|--------------|------------------|
| Small | 10 | 100K | < 30s | < 10s |
| Medium | 100 | 10M | 2-5 min | < 1 min |
| Large | 500 | 100M | 15-30 min | 2-5 min |
| Full | 1000 | 1B | 1-2 hours | 10-20 min |

**Optimization Strategy: Smart Filtering**

Instead of streaming all files for every claimed directory, we use the rule definitions to filter queries:

1. **Non-recursive rules (`recursive=0`)**: Only query the immediate directory.
   ```sql
   SELECT ... FROM fs_files WHERE dir_path = ?
   ```
2. **Recursive rules (`recursive=1`)**: Query with `startsWith`.
   ```sql
   SELECT ... FROM fs_files WHERE startsWith(dir_path, ?)
   ```
3. **Exclusion Logic**: If the state machine indicates that a large subdirectory is fully covered by `BackupNone` (and no `no_override` rules override it), we can exclude it from the query.
   - *Implementation*: Walk the state machine to find "cut-off" paths where the rule is `BackupNone` and `recursive=1`. Add `AND NOT startsWith(dir_path, ?)` to the query.

**Parallelism**: Run queries for different claimed directories in parallel (e.g., 10-20 concurrent workers).

**Key helper functions:**

```go
// streamFilesForBackup queries fs_files under the given prefix and evaluates
// each file against the state machine to determine if it should be backed up.
// This mirrors the web server's aggregation logic.
func streamFilesForBackup(ctx context.Context, conn clickhouse.Conn,
    fsRoot string, snapshotID uint32, prefix string,
    sm group.StateMachine[db.Rule], exactRules map[string]*db.Rule,
    cb func(filePath string, rule *db.Rule)) error {
    
    query := `SELECT dir_path, name FROM fs_files 
              WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
              ORDER BY dir_path, name`
    
    rows, err := conn.Query(ctx, query, fsRoot, snapshotID, prefix)
    if err != nil {
        return err
    }
    defer rows.Close()
    
    for rows.Next() {
        var dirPath, name string
        if err := rows.Scan(&dirPath, &name); err != nil {
            return err
        }
        
        fullPath := dirPath + name
        
        // Determine effective rule using same logic as aggregation
        var rule *db.Rule
        if r, ok := exactRules[fullPath]; ok {
            rule = r
        } else {
            // Build FileInfo for state machine
            // The state machine uses FileInfo.Path (DirectoryPath) and FileInfo.Name
            fi := buildFileInfo(dirPath, name)  // helper to construct DirectoryPath chain
            rule = sm.GetGroup(fi)
        }
        
        cb(fullPath, rule)  // rule may be nil for unplanned files
    }
    return rows.Err()
}
```

---

## 6. Go Interface Requirements

The new ClickHouse-based implementation must satisfy the same interfaces as the
current `ruletree.RootDir`. This ensures drop-in replacement with minimal changes
to `backend/` code.

### 6.1. RootDir Interface

```go
// RootDir provides directory summaries and ownership lookups.
// Current implementation: ruletree.RootDir (in-memory tree database)
// New implementation: CHRootDir (ClickHouse-backed with in-memory aggregates)
type RootDir interface {
    // Summary returns aggregated stats for a directory and its immediate children.
    // Path is relative to root, without leading slash (e.g., "lustre/scratch/project/")
    // Pass "" to get the root-level summary (all filesystem roots aggregated).
    Summary(path string) (*DirSummary, error)
    
    // GetOwner returns the UID and GID of a directory.
    // Path is relative to root, without leading slash.
    GetOwner(path string) (uid, gid uint32, err error)
    
    // IsDirectory checks if a path exists and is a directory.
    // Path format: with or without leading slash (implementation should handle both).
    IsDirectory(path string) bool
    
    // AddTree loads a new snapshot (replaces current for that fs_root).
    // For ClickHouse: triggers aggregate rebuild for that root.
    AddTree(file string) error
    
    // AddRule adds a rule and recomputes affected aggregates.
    AddRule(dir *db.Directory, rule *db.Rule) error
    
    // RemoveRule removes a rule and recomputes affected aggregates.
    RemoveRule(dir *db.Directory, rule *db.Rule) error
    
    // AddRules batch-adds rules (for FOFN import) with single recompute.
    AddRules(topPath string, rules []DirRule) error
}
```

**Note on AddTree:** In the current implementation, `AddTree(file)` reads a
pre-built tree database file. In the ClickHouse implementation, `AddTree` is
replaced by signaling that a new snapshot is ready:

```go
// CHRootDir implementation
func (c *CHRootDir) AddTree(file string) error {
    // In ClickHouse mode, 'file' is ignored. Instead, we:
    // 1. Query fs_snapshots to detect newly-current snapshots
    // 2. Rebuild aggregates for any root with a changed snapshot_id
    return c.RefreshSnapshots()
}

func (c *CHRootDir) RefreshSnapshots() error {
    newSnapshots := c.loadCurrentSnapshots()  // query ClickHouse
    
    for root, newID := range newSnapshots {
        if oldID, ok := c.currentSnapshots[root]; !ok || oldID != newID {
            // Snapshot changed, rebuild aggregates for this root
            if err := c.rebuildAggregatesForRoot(root, newID); err != nil {
                return err
            }
        }
    }
    
    c.currentSnapshots = newSnapshots
    return nil
}
```

The web server can call `RefreshSnapshots()` periodically (e.g., every 5 minutes)
or expose an admin endpoint to trigger refresh after `ch-import` completes.

### 6.2. DirSummary Structure

Must match the existing JSON output for frontend compatibility:

```go
type DirSummary struct {
    uid, gid      uint32           // Directory ownership
    User          string           // Username (resolved from uid)
    Group         string           // Group name (resolved from gid)
    ClaimedBy     string           // From plan DB (empty if unclaimed)
    RuleSummaries []Rule           // Stats per rule affecting this dir
    Children      map[string]*DirSummary  // Immediate child directories
}

type Rule struct {
    ID     uint64      // Rule ID (0 = unplanned/warn)
    Users  RuleStats   // Per-user file stats
    Groups RuleStats   // Per-group file stats
}

type Stats struct {
    id    uint32   // UID or GID
    Name  string   // Username or group name
    MTime uint64   // Latest mtime of files
    Files uint64   // File count
    Size  uint64   // Total size in bytes
}
```

**Rule ID 0 (Unplanned/BackupWarn):** Files not matching any rule are attributed
to rule ID 0. The frontend displays these as "Unplanned" in the summary. The
`Rules` map returned by `/api/tree` does not include an entry for ID 0; the
frontend synthesizes a pseudo-rule with `BackupType = BackupWarn` for display.

---

## 7. Implementation Checklist

### Phase 1: Infrastructure
- [ ] Set up ClickHouse (single node or cluster), create schema (section 2)
- [ ] Implement `CHClient` wrapper with connection pooling
- [ ] Add ClickHouse connection config to `cmd/config.go`

### Phase 2: Ingestion
- [ ] Implement `ch-import` command (`cmd/ch_import.go`)
- [ ] Parse stats.gz using `summary.NewSummariser` 
- [ ] Batch insert to `fs_files` and `fs_dirs` (100k rows per batch)
- [ ] Auto-populate `fs_dir_stats` via materialized view
- [ ] Handle `fs_snapshots` management (insert, mark current, cleanup old)
- [ ] Benchmark: < 1hr for 1.3B files across all roots

### Phase 3: Aggregates
- [ ] Implement `CHRootDir` struct satisfying `RootDir` interface
- [ ] Implement `buildAggregates()` - single pass over `fs_files` filtered to
    the minimal ruleDir prefixes (optionally seed catch-all rules from
    `fs_dir_stats` when filename is irrelevant)
- [ ] Implement state machine integration with `recursive` and `no_override` flags
- [ ] Implement `exactRules` hash map for non-glob rules
- [ ] Implement `recomputeSubtree()` for incremental updates on rule changes
- [ ] Benchmark: < 5 min aggregate build, < 1s rule update

### Phase 4: API Integration
- [ ] Replace `ruletree.RootDir` with `CHRootDir` in `backend.Server`
- [ ] Verify `/api/tree` returns identical JSON structure
- [ ] Verify `/api/report/summary` works correctly
- [ ] Verify `/api/usergroups` works correctly  
- [ ] Test rule CRUD operations with <1s UI refresh
- [ ] Test FOFN upload with batch rule creation

### Phase 5: Backup Command
- [ ] Implement ClickHouse-based file enumeration in `backups/` package
- [ ] Handle mixed snapshot dates across roots
- [ ] Implement parallel directory processing
- [ ] Benchmark: < 1hr, target < 10 min for full backup computation

### Phase 6: Migration & Testing
- [ ] Create migration script for existing deployments
- [ ] Side-by-side testing: verify identical results vs old implementation
- [ ] Document ClickHouse deployment and maintenance procedures
- [ ] Add snapshot cleanup cron job (keep last N days per root)

### Phase 7: UI Updates (Optional)
- [ ] Add UI controls for `recursive` flag (`*` vs `**`)
- [ ] Add UI controls for `no_override` flag (non-overridable rules)

---

## 8. Key Queries Reference

```sql
-- Get current snapshot for each root
SELECT fs_root, snapshot_id, snapshot_date
FROM fs_snapshots FINAL 
WHERE is_current = 1;

-- Filename-sensitive aggregation (single pass over relevant prefixes)
SELECT dir_path, name, uid, gid, size, mtime
FROM fs_files
WHERE (fs_root, snapshot_id) IN (
        SELECT fs_root, snapshot_id FROM fs_snapshots FINAL WHERE is_current = 1
)
    AND startsWith(dir_path, ?)   -- iterate over minimal ruleDir prefixes
ORDER BY dir_path, name;

-- Optional: seed pure catch-all rules from aggregates when filename is irrelevant
SELECT dir_path, uid, gid,
             sum(file_count) AS file_count,
             sum(total_size) AS total_size,
             max(max_mtime) AS max_mtime
FROM fs_dir_stats FINAL
WHERE (fs_root, snapshot_id) IN (
        SELECT fs_root, snapshot_id FROM fs_snapshots FINAL WHERE is_current = 1
)
    AND startsWith(dir_path, ?)
GROUP BY dir_path, uid, gid
ORDER BY dir_path;

-- Check directory exists (for IsDirectory)
SELECT 1 FROM fs_dirs 
WHERE fs_root = ? AND snapshot_id = ? AND dir_path = ? 
LIMIT 1;

-- Get directory owner (for GetOwner, CanClaim checks)
SELECT uid, gid FROM fs_dirs 
WHERE fs_root = ? AND snapshot_id = ? AND dir_path = ?;

-- Files under directory with recursive rule (for backup)
SELECT dir_path, name FROM fs_files
WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
ORDER BY dir_path, name;

-- Files in immediate directory only (non-recursive rule)
SELECT dir_path, name FROM fs_files
WHERE fs_root = ? AND snapshot_id = ? AND dir_path = ?
ORDER BY name;

-- Filename-sensitive rule recompute for a subtree
SELECT dir_path, uid, gid, 
       count() AS file_count, 
       sum(size) AS total_size, 
       max(mtime) AS max_mtime
FROM fs_files
WHERE fs_root = ? AND snapshot_id = ? 
  AND startsWith(dir_path, ?) 
  AND match(name, ?)   -- ClickHouse regex match
GROUP BY dir_path, uid, gid
ORDER BY dir_path;

-- Delete old snapshot (instantaneous via partition drop)
-- Note: With tuple partitioning, use partition ID from system.parts
-- First find partition: SELECT partition FROM system.parts WHERE table='fs_files' AND partition LIKE '%<fs_root>%<snapshot_id>%'
-- Or use explicit tuple format:
ALTER TABLE fs_files DROP PARTITION ('<fs_root_value>', <snapshot_id_value>);
ALTER TABLE fs_dirs DROP PARTITION ('<fs_root_value>', <snapshot_id_value>);
ALTER TABLE fs_dir_stats DROP PARTITION ('<fs_root_value>', <snapshot_id_value>);
DELETE FROM fs_snapshots WHERE fs_root = ? AND snapshot_id = ?;

-- Mark new snapshot as current (atomic update)
INSERT INTO fs_snapshots (fs_root, snapshot_id, snapshot_date, is_current)
VALUES (?, ?, ?, 1);
-- Old current is automatically superseded by ReplacingMergeTree when queried with FINAL
```

---

## 9. Performance Considerations

### 9.1. Ingestion Performance

- Use `clickhouse-go` native protocol for bulk inserts
- Batch size: 100,000 rows per insert
- Parallel ingestion: run separate `ch-import` processes per stats.gz file
- Disable `fsync_after_insert` during bulk load if durability allows
- Expected: ~500k rows/second per connection, ~45 min for 1.3B files

### 9.2. Query Performance

- Partition pruning eliminates old snapshots from scans
- `startsWith(dir_path, ?)` uses primary key efficiently due to ORDER BY
- `fs_dir_stats` reduces data volume ~20x vs `fs_files` for aggregation
- Materialized view populates stats during insert, no separate ETL step

### 9.3. Memory Usage

- In-memory `dirSummaries` for 10M directories: ~2-4 GB (depending on rule count)
- State machine for 10k glob rules: ~100 MB
- Exact rules map for 1M paths: ~200 MB
- Total server memory requirement: ~8 GB recommended

### 9.4. Snapshot Cleanup

Daily cron job to remove old snapshots:
```bash
backup-plans ch-cleanup --keep-days 7 --ch-dsn "..."
```

---

## 10. Edge Cases and Error Handling

### 10.1. Missing Snapshots

- The previous snapshot remains `is_current = 1` for that `fs_root`
- Other roots can have newer snapshots (mixed dates)
- UI should display snapshot dates per root to inform users of data freshness
- Backup command proceeds with available snapshots

**API Enhancement for Snapshot Visibility:**

Add a new endpoint or extend existing responses to expose snapshot dates:

```go
// GET /api/snapshots - returns current snapshot info per root
type SnapshotInfo struct {
    Snapshots map[string]time.Time `json:"snapshots"`  // fs_root -> snapshot_date
}
```

This allows the frontend to display "Data as of: /lustre/scratch123: Dec 9, /nfs/team: Dec 10"
so users understand when different parts of the filesystem were last scanned.

### 10.2. ClickHouse Unavailability

- Web server startup should fail fast if ClickHouse is unreachable
- API endpoints should return appropriate errors if aggregates aren't loaded
- Backup command should retry with exponential backoff

### 10.3. Large Rule Updates

For rule changes affecting large subtrees (100K+ directories):
- Consider async recomputation with progress indicator
- Or accept 1-5s delay for very large updates
- Frontend can poll for completion or use optimistic UI update

### 10.4. Concurrent Rule Modifications

- Use mutex around in-memory aggregate updates (as current implementation does)
- Plan DB transactions ensure rule persistence is atomic
- State machine rebuild is fast enough (~100ms) to hold lock during rebuild

### 10.5. Path Encoding

- All paths use UTF-8 encoding
- NUL bytes in paths are rejected at rule creation time
- ClickHouse String type handles arbitrary bytes but UI validation prevents issues
