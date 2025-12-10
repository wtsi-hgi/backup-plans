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
- Rules must NOT contain `/` in `match` (filename-only patterns; the directory path is stored separately)
- FOFN-imported rules are allowed to be exact filenames (no `*`) for individual paths
- In all cases `match` is interpreted relative to the directory; the effective pattern the matcher sees is `dir.Path + match`
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

### 1.4. Advanced Rule Semantics (`*` vs `**` and Priority)

To support `.gitignore`-style semantics and non-overridable rules, the rule schema includes:

```sql
ALTER TABLE rules ADD COLUMN recursive TINYINT NOT NULL DEFAULT 1;
ALTER TABLE rules ADD COLUMN priority  TINYINT NOT NULL DEFAULT 0;
```

- `recursive = 0`: `*` matches only immediate directory (non-recursive)
- `recursive = 1`: `*` matches recursively into subdirectories (current behavior, `**` equivalent)
- `priority = 0`: Normal rule (can be overridden by child rules)
- `priority = 1`: Non-overridable (takes precedence even if child rules exist)

**Migration:** Existing rules get `recursive = 1, priority = 0` to preserve behavior.
The state machine builder must be updated to respect these flags:

- First resolve by `priority` (1 before 0)
- Then apply the existing "most specific directory path wins" rule among matches of the same priority
- `recursive = 0` patterns must *not* match descendants; the UI can expose this as `*` vs `**` semantics.

**State Machine Modifications:**

The current `group.StateMachine` from `wrstat-ui` treats `*` as matching `/` (recursive).
To support non-recursive matching:

1. For rules with `recursive = 0`, replace `*` with a pattern that matches any character
   except `/` before adding to the state machine. This makes `*` behave like shell globbing.
   
2. For rules with `recursive = 1` (default, current behavior), keep `*` matching `/`.

3. For `priority = 1` rules, the state machine's "most specific wins" logic must be
   extended to check priority first. Implementation options:
   
   a. **Separate state machines**: Build two state machines (high/low priority),
      check high-priority first.
   
   b. **Modified group resolution**: Extend `group.PathGroup` to include priority,
      modify `GetGroup` to prefer higher priority matches.
   
   Option (a) is simpler and avoids modifying the external `wrstat-ui` package.

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

**Usage:** Fast path for rules whose match is effectively "match everything"
in a subtree (e.g. `*`/`**`) and for `IsDirectory`/ownership checks. It is
*not* sufficient for filename-specific patterns (e.g. `*.bam`, `temp-*`); those
still need per-file basenames from `fs_files` (see section 4.2).

### 2.5. Snapshot Lifecycle

**Add snapshot:** Import to `fs_files`/`fs_dirs`, mark current via INSERT to `fs_snapshots`.

**Delete snapshot:** `ALTER TABLE ... DROP PARTITION (fs_root, snapshot_id)` - instantaneous.

**Mixed dates:** Each `fs_root` tracks its own `is_current` snapshot independently.
When a stats.gz file for a particular filesystem is not generated on a given day,
the system continues using the most recent snapshot for that root while other
roots may have newer snapshots. The web server and backup command must:
1. Query `fs_snapshots FINAL WHERE is_current = 1` to get the active snapshot per root
2. Join or filter queries by `(fs_root, snapshot_id)` pairs from step 1

---

## 3. Ingestion

### 3.1. Command: `backup-plans ch-import`

```bash
backup-plans ch-import --stats /path/to/stats.gz --ch-dsn "clickhouse://..." [--mark-current]
```

### 3.2. Algorithm

1. Open `stats.gz`, parse with `summary.NewSummariser`
2. Batch insert to `fs_files` and `fs_dirs` (flush every 100k rows)
3. Register snapshot in `fs_snapshots`
4. If `--mark-current`, set `is_current = 1`

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

- **`dirSummaries map[string]*DirSummary`** - Recursive stats per directory,
  built by walking `fs_dir_stats` and applying the state machine. Each summary
    contains per-rule aggregates broken down by user/group. The structure is
    intentionally compatible with the existing `ruletree.DirSummary` so that
    `/api/tree`, `/api/report/summary`, and `/api/usergroups` can keep their
    JSON shapes unchanged.

### 4.2. Building Aggregates

At startup or when a new snapshot becomes current:

1. **Load current snapshots** - Query `fs_snapshots FINAL WHERE is_current = 1` to get
   active `(fs_root, snapshot_id)` pairs. Store in `currentSnapshots map[string]uint32`.

2. **Rebuild state machine** from plan DB rules. Each rule becomes a
   `PathGroup{Path: dir.Path + rule.Match, Group: rule}`. The state machine
   compiles these into an automaton. Also populate `exactRules map[string]*db.Rule`
   for rules without `*` in their match pattern.

3. **Single-pass aggregation**: Stream all `fs_dir_stats` rows for current snapshots,
   ordered by `dir_path`. For each `(dir_path, uid, gid, file_count, total_size, max_mtime)`:
   
   a. Determine the winning rule by evaluating the state machine with `dir_path + "*"`
      (catch-all simulation). This gives the rule that would apply to files in this dir.
   
   b. Look up/create `DirSummary` for `dir_path`
   
   c. Update `RuleSummaries[ruleID].Users[uid]` and `RuleSummaries[ruleID].Groups[gid]`

4. **Handle filename-sensitive rules**: For rules with non-`*` patterns (e.g., `*.bam`, `temp-*`):
   - Query `fs_files` for the rule's directory subtree
   - Apply the glob pattern to `name` in Go
   - Aggregate matching files into separate RuleSummary entries
   - Subtract these from the catch-all rule's totals to avoid double-counting

5. **Aggregate upward**: Process directories deepest-first (reverse `dir_path` order).
   For each directory, add its per-rule stats into all ancestors.

**Performance:** ~2-5 min for 50M aggregated rows, depending on rule count
and tree depth.

### 4.3. `/api/tree` Implementation

Pure memory lookup + JSON serialization from `dirSummaries` and
`directoryRules` (plan DB). **Latency:** < 10ms.

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

**Authorization logic** (same as current implementation):
- User is authorized if: `uid == dir.uid` OR `gid in user.groups` OR `user.groups contains adminGroup`
  OR user owns files within the subtree (check via RuleSummaries)
- `CanClaim` is true if user is owner or in directory's group
- `Unauthorised` lists child directory names where user lacks authorization

### 4.4. Incremental Updates

On rule create/update/remove:

1. **Persist to plan DB** - Insert/update/delete the rule row.

2. **Rebuild state machine** (~100ms for thousands of rules) - The entire
   automaton is rebuilt since rule changes can affect pattern priority.
   Also rebuild `exactRules` map.

3. **Re-query the affected subtree**
   - Identify the minimum subtree that could be affected: the rule's directory path
   - For catch-all rules: re-aggregate from `fs_dir_stats` for that subtree
   - For filename-sensitive rules: stream `fs_files` for the subtree and apply
     the rule glob to `name`, grouping to the same `(dir_path, uid, gid)` shape

4. **Subtract old rule's contribution, add new rule's contribution** in memory.
   This avoids full re-aggregation.

5. **Propagate to ancestors** - Recompute ancestor aggregates by summing children.

**Latency:** < 100ms for small directories, 1-5s for large project roots
(100K+ subdirectories).

### 4.5. `/api/report/summary` Implementation

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

### 4.6. Exact-Path Rules at Scale

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

### 4.7. `/api/usergroups` Implementation

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
    
    // 4. For each claimed directory with BackupIBackup rules
    for _, dir := range dirs {
        for _, rule := range getRulesForDir(planDB, dir.ID()) {
            if rule.BackupType != db.BackupIBackup {
                continue
            }
            
            // Determine which snapshot covers this directory
            fsRoot, snapshotID := findSnapshot(snapshots, dir.Path)
            if snapshotID == 0 {
                continue  // No snapshot for this root
            }
            
            // Stream matching files
            streamFiles(ctx, chConn, fsRoot, snapshotID, dir.Path, rule,
                func(filePath string) {
                    // Verify this file matches our rule (not overridden)
                    if matchingRule := getEffectiveRule(exactRules, sm, filePath); 
                       matchingRule != nil && matchingRule.ID() == rule.ID() {
                        setFofns[dir.ID()] = append(setFofns[dir.ID()], filePath)
                    }
                })
        }
    }
    
    return submitToIBackup(client, setFofns, dirs)
}
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
3. **Exclusion Logic**: If the state machine indicates that a large subdirectory is fully covered by `BackupNone` (and no high-priority rules override it), we can exclude it from the query.
   - *Implementation*: Walk the state machine to find "cut-off" paths where the rule is `BackupNone` and `recursive=1`. Add `AND NOT startsWith(dir_path, ?)` to the query.

**Parallelism**: Run queries for different claimed directories in parallel (e.g., 10-20 concurrent workers).

**Key helper functions:**

```go
// streamFiles queries fs_files for files under dirPath matching the rule pattern
func streamFiles(ctx context.Context, conn clickhouse.Conn, fsRoot string, 
    snapshotID uint32, dirPath string, rule *db.Rule, 
    cb func(filePath string)) error {
    
    var query string
    if rule.Recursive {
        query = `SELECT dir_path, name FROM fs_files 
                 WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)`
    } else {
        query = `SELECT dir_path, name FROM fs_files 
                 WHERE fs_root = ? AND snapshot_id = ? AND dir_path = ?`
    }
    
    rows, err := conn.Query(ctx, query, fsRoot, snapshotID, dirPath)
    if err != nil { return err }
    defer rows.Close()
    
    pattern := compileGlob(rule.Match)  // e.g., "*.bam" -> regex
    
    for rows.Next() {
        var dp, name string
        rows.Scan(&dp, &name)
        if pattern.MatchString(name) {
            cb(dp + name)
        }
    }
    return nil
}

// getEffectiveRule returns the rule that applies to a file path
func getEffectiveRule(exactRules map[string]*db.Rule, 
    sm group.StateMachine[db.Rule], filePath string) *db.Rule {
    if rule, ok := exactRules[filePath]; ok {
        return rule
    }
    return sm.GetGroup(buildFileInfo(filePath))
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
    // Path is relative to root (e.g., "lustre/scratch/project/")
    Summary(path string) (*DirSummary, error)
    
    // GetOwner returns the UID and GID of a directory.
    GetOwner(path string) (uid, gid uint32, err error)
    
    // IsDirectory checks if a path exists and is a directory.
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
- [ ] Implement `buildAggregates()` - single pass over `fs_dir_stats`
- [ ] Implement state machine integration with `recursive` and `priority` flags
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
- [ ] Add UI controls for `priority` flag (non-overridable rules)

---

## 8. Key Queries Reference

```sql
-- Get current snapshot for each root
SELECT fs_root, snapshot_id, snapshot_date
FROM fs_snapshots FINAL 
WHERE is_current = 1;

-- Directory stats for aggregation (single pass)
SELECT dir_path, uid, gid, 
       sum(file_count) AS file_count, 
       sum(total_size) AS total_size, 
       max(max_mtime) AS max_mtime
FROM fs_dir_stats FINAL 
WHERE (fs_root, snapshot_id) IN (
    SELECT fs_root, snapshot_id FROM fs_snapshots FINAL WHERE is_current = 1
)
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
ALTER TABLE fs_files DROP PARTITION (?, ?);
ALTER TABLE fs_dirs DROP PARTITION (?, ?);
ALTER TABLE fs_dir_stats DROP PARTITION (?, ?);
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
# Keep only snapshots from last 7 days per root
backup-plans ch-cleanup --keep-days 7 --ch-dsn "..."
```

---

## 10. Edge Cases and Error Handling

### 10.1. Missing Snapshots

If a stats.gz file is not generated for a filesystem on a given day:
- The previous snapshot remains `is_current = 1` for that `fs_root`
- Other roots can have newer snapshots (mixed dates)
- UI should display snapshot dates per root to inform users of data freshness
- Backup command proceeds with available snapshots

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
