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

- Must contain `*` (wildcard)
- Must NOT contain `/` (filename-only patterns)
- `*` matches recursively into subdirectories (via `group.StateMachine`)

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
The state machine builder must be updated to respect these flags.

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

**Usage:** This is the primary data source for building in-memory aggregates,
avoiding full scans of the 1.3B-row `fs_files` table.

### 2.5. Snapshot Lifecycle

**Add snapshot:** Import to `fs_files`/`fs_dirs`, mark current via INSERT to `fs_snapshots`.

**Delete snapshot:** `ALTER TABLE ... DROP PARTITION (fs_root, snapshot_id)` - instantaneous.

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
  contains per-rule aggregates broken down by user/group.

### 4.2. Building Aggregates

At startup or when a new snapshot becomes current:

1. **Rebuild state machine** from plan DB rules. Each rule becomes a
   `PathGroup{Path: dir.Path + rule.Match, Group: rule}`. The state machine
   compiles these into an automaton.

2. **Query `fs_dir_stats`** ordered by `dir_path`. The ordering ensures we
   process directories in tree order (parents before deeply nested children).

3. **For each directory row**, use the state machine to determine which rule
   applies to files in that directory. The state machine is evaluated with
   the directory path to get the effective rule.

4. **Aggregate upward**: Add each directory's stats to all its ancestors.
   Maintain a stack of ancestor builders; when the path prefix changes,
   pop and finalize completed directories.

**Performance:** ~2-5 min for 50M aggregated rows, depending on rule count
and tree depth.

### 4.3. `/api/tree` Implementation

Pure memory lookup + JSON serialization. **Latency:** < 10ms

### 4.4. Incremental Updates

On rule create/update/remove:

1. **Persist to plan DB** - Insert/update/delete the rule row.

2. **Rebuild state machine** (~100ms for thousands of rules) - The entire
   automaton is rebuilt since rule changes can affect pattern priority.

3. **Re-query `fs_dir_stats`** for the affected subtree only:
   ```sql
   SELECT dir_path, uid, gid, file_count, total_size, max_mtime
   FROM fs_dir_stats FINAL
   WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
   ORDER BY dir_path
   ```

4. **Propagate to ancestors** - Update parent directories' aggregates to
   reflect the changed rule assignments.

**Latency:** < 100ms for small directories, 1-5s for large project roots
(100K+ subdirectories).

---

## 5. Backup Job

The `backup` command runs statelessly on a separate machine with no in-memory cache.

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
func Backup(ctx context.Context, planDB *db.DB, chConn clickhouse.Conn,
    fsRoot string, snapshotID uint32, client *ibackup.MultiClient) ([]SetInfo, error) {
    
    dirs := loadDirectories(planDB)
    sm, dirPaths := buildStateMachine(planDB, dirs)
    setFofns := make(map[int64][]string)

    for dirPath := range dirPaths {
        rows, _ := chConn.Query(ctx, `
            SELECT dir_path, name FROM fs_files
            WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
        `, fsRoot, snapshotID, dirPath)
        
        for rows.Next() {
            var fileDirPath, fileName string
            rows.Scan(&fileDirPath, &fileName)
            
            fi := buildFileInfo(fileDirPath, fileName)
            rule := sm.GetGroup(fi)
            if rule != nil && rule.BackupType == db.BackupIBackup {
                setFofns[rule.DirID()] = append(setFofns[rule.DirID()], fileDirPath+fileName)
            }
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

---

## 6. Implementation Checklist

### Phase 1: Infrastructure
- [ ] Set up ClickHouse, create schema (section 2)
- [ ] Implement `CHClient` wrapper

### Phase 2: Ingestion
- [ ] Implement `ch-import` command
- [ ] Benchmark: < 1hr for 1.3B files

### Phase 3: Aggregates
- [ ] Implement `buildAggregates()` with state machine
- [ ] Implement `recomputeSubtree()` for incremental updates
- [ ] Benchmark: < 5 min aggregate build

### Phase 4: API
- [ ] Implement `CHRootDir` (same interface as current `RootDir`)
- [ ] Verify `/api/tree` < 1s, rule changes < 1s refresh

### Phase 5: Backup
- [ ] Implement ClickHouse-based backup enumeration
- [ ] Benchmark: < 1hr (target < 10 min)

### Phase 6: Migration
- [ ] Test identical results vs old implementation
- [ ] Document migration procedure

### Phase 7: UI Updates
- [ ] Add UI controls for recursive/priority flags

---

## 7. Key Queries Reference

```sql
-- Current snapshot
SELECT * FROM fs_snapshots FINAL WHERE fs_root = ? AND is_current = 1;

-- Directory stats for aggregation
SELECT dir_path, uid, gid, sum(file_count), sum(total_size), max(max_mtime)
FROM fs_dir_stats FINAL WHERE fs_root = ? AND snapshot_id = ?
GROUP BY dir_path, uid, gid ORDER BY dir_path;

-- Check directory exists
SELECT 1 FROM fs_dirs WHERE fs_root = ? AND snapshot_id = ? AND dir_path = ? LIMIT 1;

-- Get directory owner
SELECT uid, gid FROM fs_dirs WHERE fs_root = ? AND snapshot_id = ? AND dir_path = ?;

-- Files under directory (for backup)
SELECT dir_path, name FROM fs_files
WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
ORDER BY dir_path, name;

-- Delete old snapshot (instantaneous)
ALTER TABLE fs_files DROP PARTITION (?, ?);
ALTER TABLE fs_dirs DROP PARTITION (?, ?);
ALTER TABLE fs_dir_stats DROP PARTITION (?, ?);
```
