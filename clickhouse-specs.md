# ClickHouse Implementation Specs for Backup Plans

This document specifies how to replace the current custom `treegen`/`ruletree`
database with a ClickHouse-backed implementation, while keeping the Go
interfaces (`backend`, `ruletree`, `backups`) behaving the same.

## Executive Summary

### Scale and Performance Requirements

| Metric | Current Value | Requirement |
|--------|---------------|-------------|
| Total files per snapshot | ~1.3 billion | Must handle |
| Glob-pattern rules | Hundreds, up to thousands | < 1s rule evaluation |
| Exact-path rules | Could be millions | < 1s batch matching |
| Filesystems (roots) | Multiple (e.g., 5-10) | Independent snapshot dates |
| `/api/tree` latency | N/A | < 1s |
| Rule add/update with UI refresh | N/A | < 1s |
| Full backup computation | ~minutes currently | < 1hr, target few minutes |
| Database refresh (per root) | Daily | < 1hr per root |
| Old data deletion | After new data ready | < 1hr |

### Key Design Decisions

1. **ClickHouse stores filesystem stats only** - the plan DB (SQLite/MySQL)
   remains the source of truth for rules and directory claims.
2. **Per-root snapshots** - each filesystem root has independent snapshots;
   missing daily `stats.gz` files don't block other roots.
3. **In-memory aggregates for interactive queries** - `/api/tree` uses
   pre-computed in-memory structures, not live ClickHouse queries.
4. **State machine for rule matching** - reuse `wrstat-ui/summary/group`
   automaton for O(path-length) rule evaluation.
5. **Incremental updates** - rule changes trigger partial re-aggregation,
   not full snapshot re-processing.

---

## 1. Behavioural Model and Required Queries

### 1.1. Rules and Their Semantics

#### 1.1.1. Rule Storage (Plan DB)

Rules are stored in the existing SQL plan database (`db` package), which uses
SQLite or MySQL. The schema consists of two tables:

**`directories` table:**
```sql
CREATE TABLE directories (
    id          INTEGER PRIMARY KEY,
    path        TEXT NOT NULL UNIQUE,  -- always ends with '/'
    claimed_by  TEXT NOT NULL,         -- username of claimant
    frequency   INTEGER NOT NULL,      -- backup frequency in days
    review_date INTEGER NOT NULL,      -- Unix timestamp
    remove_date INTEGER NOT NULL,      -- Unix timestamp
    created     INTEGER NOT NULL,
    modified    INTEGER NOT NULL
);
```

**`rules` table:**
```sql
CREATE TABLE rules (
    id           INTEGER PRIMARY KEY,
    directory_id INTEGER NOT NULL REFERENCES directories(id),
    backup_type  INTEGER NOT NULL,     -- enum: 0=None, 1=IBackup, 2-5=Manual*
    metadata     TEXT NOT NULL,        -- for manual types: "requester:name"
    match_str    TEXT NOT NULL,        -- glob pattern, e.g. "*.txt", "*"
    created      INTEGER NOT NULL,
    modified     INTEGER NOT NULL
);
```

**BackupType enum values:**
```go
const (
    BackupNone BackupType = iota        // 0: Do not back up
    BackupIBackup                       // 1: Automatic backup via ibackup
    BackupManualIBackup                 // 2: Manual ibackup set reference
    BackupManualGit                     // 3: Manual git-based backup
    BackupManualUnchecked               // 4: Manual unchecked backup
    BackupManualPrefect                 // 5: Manual Prefect workflow backup
)
```

#### 1.1.2. Match String Constraints

The `Match` field (called `match_str` in SQL) has these constraints enforced
by `backend/rules.go:validMatch()`:

- **Must contain at least one `*`** (wildcard character)
- **Must NOT contain `/`** (patterns are filename-only, not path segments)
- **Must NOT contain NUL (`\x00`)** bytes
- Empty `Match` is normalized to `*`
- `Match` ending in `/` is normalized to `<match>*` (e.g., `foo/` -> `foo*`)

**Valid examples:** `*`, `*.txt`, `temp-*`, `*.tar.gz`, `file_*_backup`
**Invalid examples:** `foo/bar`, `**/*.txt` (contains `/`), `no-wildcard`

#### 1.1.3. Current `*` Semantics (Recursive by Default)

The current implementation uses `wrstat-ui/summary/group.StateMachine` for
pattern matching. The state machine treats `*` as matching:
- Zero or more characters in the filename, AND
- All files in subdirectories recursively (i.e., `*` behaves like `**`)

**Example:** Rule on `/data/project/` with `Match = "*.txt"`:
- Matches: `/data/project/file.txt`
- Matches: `/data/project/subdir/deep/file.txt` (recursive!)
- Does NOT match: `/data/project/file.csv`

This recursive behavior means a rule on a parent directory applies to the
entire subtree unless overridden by a more specific rule on a child directory.

#### 1.1.4. Effective Rule Determination

For any file at path `P`, the effective rule is determined as:

1. **Find all candidate rules:** rules whose `directory.path` is a prefix of
   the file's parent directory path.
2. **Filter by pattern match:** keep only rules where the file's basename
   matches the `Match` glob pattern.
3. **Select most specific:** choose the rule with the longest `directory.path`.
4. **Default to "unplanned":** if no rule matches, the file is "unplanned"
   (internal rule ID = 0, displayed as "BackupWarn" in the UI).

**Example:**
```
Rules:
  /data/project/*           -> BackupIBackup  (rule ID 1)
  /data/project/temp-*      -> BackupNone     (rule ID 2)  
  /data/project/archive/*.gz -> BackupIBackup  (rule ID 3)

Files and their effective rules:
  /data/project/file.txt           -> rule 1 (matches *, BackupIBackup)
  /data/project/temp-cache.dat     -> rule 2 (matches temp-*, BackupNone)
  /data/project/archive/data.gz    -> rule 3 (matches *.gz, BackupIBackup)
  /data/project/archive/readme.txt -> rule 1 (matches * from parent)
  /other/path/file.txt             -> rule 0 (unplanned)
```

#### 1.1.5. Future Extension: `*` vs `**` and Non-Overridable Rules

To support `.gitignore`-style semantics, the rule schema will be extended:

```sql
ALTER TABLE rules ADD COLUMN recursive TINYINT NOT NULL DEFAULT 1;
ALTER TABLE rules ADD COLUMN priority  TINYINT NOT NULL DEFAULT 0;
```

- `recursive = 0`: `*` matches only the immediate directory (like `.gitignore`)
- `recursive = 1`: `*` matches recursively into subdirectories (current behavior)
- `priority = 0`: Normal rule (can be overridden by more specific child rules)
- `priority = 1`: Non-overridable (takes precedence even if child rules exist)

**Migration:** Existing rules get `recursive = 1, priority = 0` to preserve
current behavior.

**State Machine Changes:**

For non-recursive `*` (recursive = 0):
- When building the state machine, do NOT propagate the rule match into
  subdirectory prefixes
- A file `/data/project/subdir/file.txt` would NOT match rule on
  `/data/project/*.txt` if recursive = 0

For non-overridable rules (priority = 1):
- When determining effective rule, check if any ancestor rule has priority = 1
- If so, that rule takes precedence regardless of more specific child rules
- Use case: "These files must NEVER be backed up, no matter what child rules say"

### 1.2. Tree View (`/api/tree`)

The Tree View is the primary interface for browsing directories and their
backup status. It shows aggregated statistics for all files in a directory's
subtree, broken down by rule.

#### 1.2.1. API Request

```
GET /api/tree?dir=/path/to/directory/
```

The `dir` parameter must:
- Be an absolute path starting with `/`
- End with `/`
- Correspond to an existing directory in the current snapshot

#### 1.2.2. API Response Structure

```typescript
interface Tree {
    // Aggregated rule statistics for this directory and all descendants
    RuleSummaries: RuleSummary[];
    
    // Immediate child directories with their own summaries
    Children: Record<string, DirSummary>;
    
    // If this directory is claimed in the plan DB
    ClaimedBy: string;           // username or ""
    Frequency: number;           // backup frequency in days
    ReviewDate: number;          // Unix timestamp
    RemoveDate: number;          // Unix timestamp
    
    // Map of rule definitions keyed by directory path, then rule ID
    Rules: Record<string, Record<number, Rule>>;
    
    // Child directory names the user cannot access
    Unauthorised: string[];
    
    // Whether the current user can claim this directory
    CanClaim: boolean;
}

interface RuleSummary {
    ID: number;          // rule ID (0 = unplanned)
    Users: Stats[];      // per-user statistics
    Groups: Stats[];     // per-group statistics
}

interface Stats {
    ID: number;          // uid or gid
    Name: string;        // resolved username or group name
    MTime: number;       // max mtime of matching files (Unix timestamp)
    Files: number;       // count of matching files
    Size: number;        // total bytes of matching files
}

interface DirSummary {
    RuleSummaries: RuleSummary[];
    Children: Record<string, DirSummary>;  // not populated for /api/tree
    ClaimedBy: string;
}

interface Rule {
    BackupType: number;  // 0-5 enum
    Metadata: string;    // requester:name for manual types
    Match: string;       // glob pattern
}
```

#### 1.2.3. Frontend Data Flow

The frontend (`data.ts`) transforms the API response:

1. **Aggregate by BackupType:** For each rule, look up its `BackupType` and
   sum file counts/sizes into action buckets (Warn, None, IBackup, Manual*).

2. **Apply user/group filters:** If the user has selected specific users or
   groups in the filter panel, only include stats from matching `Stats` entries.

3. **Build "rules affecting subtree" tree:** Extract all directory paths from
   the `Rules` map that have non-zero matching files, and render them as a
   navigable tree.

4. **Show rule table:** For the current directory's rules (from `Rules[dir]`),
   display Match, BackupType, file counts, and edit/remove buttons.

#### 1.2.4. Authorization Logic

A user can view a directory if any of these conditions hold:
- User's UID matches the directory's UID
- User is in a group matching the directory's GID
- User is in the admin group (configurable)
- User's UID appears in any rule's `RuleSummary.Users`
- User's groups intersect with any rule's `RuleSummary.Groups`

### 1.3. Rule Editing Endpoints

#### 1.3.1. Claim Directory

```
GET /api/dir/claim?dir=/path/to/directory/
```

**Preconditions:**
- Directory exists in the current snapshot
- User's UID matches directory UID, OR user is in directory's GID group
- Directory is not already claimed

**Effects:**
- Creates a `directories` row in the plan DB
- Sets default values: `Frequency = 7`, `ReviewDate = now + 2 years`,
  `RemoveDate = ReviewDate + 30 days`

#### 1.3.2. Create Rule

```
GET /api/rules/create?dir=/path/&action=backup&match=*.txt&metadata=
```

**Parameters:**
- `dir`: claimed directory path (must end with `/`)
- `action`: one of `nobackup`, `backup`, `manualibackup`, `manualgit`,
  `manualprefect`, `manualunchecked`
- `match`: glob pattern (validated by `validMatch`)
- `metadata`: for manual types, the requester:name reference

**Preconditions:**
- Directory is claimed by the current user
- No rule with same `Match` already exists for this directory

**Effects:**
- Creates a `rules` row in the plan DB
- Updates in-memory rule structures
- Triggers partial re-aggregation for the directory's subtree
- Next `/api/tree` call reflects new rule's file counts

#### 1.3.3. Update Rule

```
GET /api/rules/update?dir=/path/&action=nobackup&match=*.txt&metadata=
```

Same as create, but the rule with matching `Match` must already exist.
Only `action` and `metadata` can be changed; `Match` is immutable.

#### 1.3.4. Remove Rule

```
GET /api/rules/remove?dir=/path/&match=*.txt
```

**Effects:**
- Deletes the `rules` row
- Updates in-memory structures
- Triggers partial re-aggregation
- Files previously matching this rule now match parent rules or become unplanned

#### 1.3.5. FOFN Upload

```
POST /api/uploadfofn?dir=/path/&action=backup&metadata=
Body: ["file1.txt", "subdir/file2.txt", "*.log"]
```

Bulk rule creation from a "file of filenames" (FOFN). Each entry is either:
- An exact filename -> creates rule with that filename as `Match`
- A glob pattern -> creates rule with that pattern as `Match`
- A path with `/` -> splits into directory + filename, may claim intermediate dirs

**Limit:** Up to 100 entries per request.

### 1.4. Global Report (`/api/report/summary`)

The report provides an overview of all claimed directories under configured
"report roots" (e.g., `/lustre/scratch123/humgen/projects/`).

#### 1.4.1. Response Structure

```typescript
interface ReportSummary {
    // Per-root summaries
    Summaries: Record<string, ClaimedDir>;
    
    // All rules referenced by any summary
    Rules: Record<number, Rule>;
    
    // Directory path -> list of rule IDs
    Directories: Record<string, number[]>;
    
    // ibackup status per directory
    BackupStatus: Record<string, BackupStatus>;
}

interface ClaimedDir extends DirSummary {
    User: string;      // resolved username from UID
    Group: string;     // resolved group name from GID
    ClaimedBy: string; // claimant username
    Children: Record<string, ClaimedDir>;  // claimed subdirectories
}

interface BackupStatus {
    LastSuccess: string;  // ISO timestamp
    Name: string;         // ibackup set name
    Requester: string;    // who requested the backup
    Failures: number;     // recent failure count
}
```

#### 1.4.2. Aggregation Logic

For each report root `R`:
1. Compute `DirSummary` for `R` (all files under `R`)
2. Find all claimed directories `D` where `D.Path` starts with `R`
3. For each such `D`, compute its `DirSummary` and attach as `Children[D.Path]`
4. Query ibackup API for backup status of each claimed directory

### 1.5. Backup Generation (`backups.Backup`)

The backup job identifies all files that should be backed up and submits them
to the ibackup service.

#### 1.5.1. Algorithm (Current Implementation)

```go
func Backup(planDB *db.DB, treeNode tree.Node, client *ibackup.MultiClient) {
    // 1. Load all directories and rules from plan DB
    dirs := loadDirectories(planDB)
    rules := loadRules(planDB)
    
    // 2. Build state machine for rule matching
    sm := group.NewStatemachine(buildRuleGroups(dirs, rules))
    
    // 3. For each file in the tree
    for file := range walkFiles(treeNode) {
        rule := sm.GetGroup(file)
        if rule != nil && rule.BackupType == BackupIBackup {
            // Add to FOFN for this rule's directory
            addToFOFN(rule.DirID, file.Path)
        }
    }
    
    // 4. Submit each FOFN to ibackup
    for dirID, files := range fofns {
        dir := dirs[dirID]
        client.Backup(
            dir.Path,                    // directory
            "plan::" + dir.Path,         // set name
            dir.ClaimedBy,               // requester
            files,                       // file list
            dir.Frequency,
            dir.ReviewDate,
            dir.RemoveDate,
        )
    }
}
```

#### 1.5.2. Performance Considerations

- Walking 1.3B files sequentially takes ~10-30 minutes depending on I/O
- State machine evaluation is O(path-length) per file
- ibackup API calls can be parallelized per directory
- Target: complete full backup computation in < 1 hour, ideally < 10 minutes

### 1.6. Helper Endpoints

#### 1.6.1. Directory Existence Check

```
POST /api/getDirectories
Body: ["/path/one/", "/path/two/"]
Response: [true, false]
```

Used by FOFN validation to check which paths are actual directories.

#### 1.6.2. Directory Ownership

Internal method `RootDir.GetOwner(path)` returns `(uid, gid, error)` for
permission checks during claim operations.

#### 1.6.3. User/Group Lists

```
GET /api/usergroups
Response: {
    Users: ["user1", "user2", ...],
    Groups: ["group1", "group2", ...],
    Owners: {"owner1": ["group1", "group2"], ...},
    BOM: {"bom1": ["group1", "group2"], ...}
}
```

Provides filter autocomplete data. Users and groups are extracted from the
current snapshot's rule summaries.

---

## 2. ClickHouse Schema

The ClickHouse schema is snapshot-oriented **per filesystem root**. Each
`stats.gz` file corresponds to one filesystem mounted at some root path (e.g.,
`/lustre/scratch123/humgen/`). For each such root we may have multiple
snapshots over time, and on any given day some roots may have a fresh snapshot
while others are still using an older one; the backend must tolerate mixed
snapshot dates across roots transparently.

### 2.1. Table: `fs_snapshots`

Tracks snapshots per filesystem root and allows quick switching or
backfilling.

```sql
CREATE TABLE fs_snapshots (
    fs_root       String,           -- e.g., '/lustre/scratch123/humgen/'
    snapshot_id   UInt32,           -- monotonically increasing per fs_root
    snapshot_date Date,             -- logical date of the stats.gz run
    file_count    UInt64 DEFAULT 0, -- total files in this snapshot
    total_size    UInt64 DEFAULT 0, -- total bytes in this snapshot
    created_at    DateTime DEFAULT now(),
    is_current    UInt8 DEFAULT 0   -- 1 = active snapshot for this root
)
ENGINE = ReplacingMergeTree(created_at)
ORDER BY (fs_root, snapshot_id);
```

**Usage patterns:**
- At most one row per `fs_root` should have `is_current = 1`
- Query current snapshot: 
  `SELECT * FROM fs_snapshots FINAL WHERE fs_root = ? AND is_current = 1`
- Switch to new snapshot: UPDATE via INSERT with new `is_current` values
- If a day's `stats.gz` is missing, the previous snapshot remains current

### 2.2. Table: `fs_files`

Raw per-file inventory for each snapshot. This is the largest table.

```sql
CREATE TABLE fs_files (
    fs_root     String,
    snapshot_id UInt32,
    dir_path    String,        -- directory path, always ends with '/'
    name        String,        -- filename only, no '/'
    size        UInt64,
    mtime       DateTime64(3), -- file modification time
    uid         UInt32,
    gid         UInt32
)
ENGINE = MergeTree()
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path, name)
SETTINGS index_granularity = 8192;
```

**Storage estimates for 1.3B files:**
- Average row size: ~150 bytes (paths dominate)
- Uncompressed: ~195 GB
- With LZ4 compression: ~40-60 GB typical
- With ZSTD compression: ~25-40 GB typical

**Key properties:**
- `dir_path + name` reconstructs the full absolute path
- Partitioned by `(fs_root, snapshot_id)` for efficient snapshot deletion
- Ordered by path for efficient prefix scans

### 2.3. Table: `fs_dirs`

Directory metadata. Used for `IsDirectory` and `GetOwner` checks.

```sql
CREATE TABLE fs_dirs (
    fs_root     String,
    snapshot_id UInt32,
    dir_path    String,        -- always ends with '/'
    uid         UInt32,        -- directory owner UID
    gid         UInt32,        -- directory owner GID
    mtime       DateTime64(3)  -- directory mtime
)
ENGINE = MergeTree()
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path);
```

**Storage estimate:** ~5-10 million directories -> ~1-2 GB compressed

### 2.4. Table: `fs_dir_stats`

Directory-level aggregated statistics. Each row summarizes all files in a
single directory (not recursive), grouped by UID and GID.

```sql
CREATE TABLE fs_dir_stats (
    fs_root     String,
    snapshot_id UInt32,
    dir_path    String,
    uid         UInt32,        -- file UID (not directory UID)
    gid         UInt32,        -- file GID
    file_count  UInt64,
    total_size  UInt64,
    max_mtime   DateTime64(3)
)
ENGINE = SummingMergeTree((file_count, total_size))
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, dir_path, uid, gid);
```

**Materialized view for automatic population:**
```sql
CREATE MATERIALIZED VIEW fs_dir_stats_mv TO fs_dir_stats AS
SELECT
    fs_root,
    snapshot_id,
    dir_path,
    uid,
    gid,
    count()       AS file_count,
    sum(size)     AS total_size,
    max(mtime)    AS max_mtime
FROM fs_files
GROUP BY fs_root, snapshot_id, dir_path, uid, gid;
```

**Usage:** This is the primary data source for building in-memory aggregates.
Instead of scanning 1.3B file rows, we scan ~50-100M aggregated rows (one per
directory/uid/gid combination).

### 2.5. Table: `fs_rule_dir_cache` (Optional)

Pre-computed per-rule, per-directory statistics for heavy operations.

```sql
CREATE TABLE fs_rule_dir_cache (
    fs_root     String,
    snapshot_id UInt32,
    rule_id     UInt64,        -- 0 = unplanned
    dir_path    String,        -- directory where stats are rooted
    uid         UInt32,        -- file UID
    gid         UInt32,        -- file GID
    file_count  UInt64,
    total_size  UInt64,
    max_mtime   DateTime64(3)
)
ENGINE = SummingMergeTree((file_count, total_size))
PARTITION BY (fs_root, snapshot_id)
ORDER BY (fs_root, snapshot_id, rule_id, dir_path, uid, gid);
```

**Population:** Filled by an offline aggregation job (see section 4.2) that
applies the rule state machine to `fs_dir_stats`.

**Usage:** Enables fast report generation and backup planning without
re-scanning all files.

### 2.6. Snapshot Lifecycle and Deletion

**Adding a new snapshot:**
1. Import data into `fs_files` and `fs_dirs` with new `snapshot_id`
2. `fs_dir_stats` is populated automatically by the materialized view
3. Run the aggregation job to populate `fs_rule_dir_cache`
4. Mark the new snapshot as current:
   ```sql
   INSERT INTO fs_snapshots (fs_root, snapshot_id, is_current)
   VALUES (?, old_id, 0), (?, new_id, 1);
   ```
5. Notify the backend to reload in-memory structures

**Deleting old snapshots:**
```sql
ALTER TABLE fs_files DROP PARTITION (fs_root, old_snapshot_id);
ALTER TABLE fs_dirs DROP PARTITION (fs_root, old_snapshot_id);
ALTER TABLE fs_dir_stats DROP PARTITION (fs_root, old_snapshot_id);
ALTER TABLE fs_rule_dir_cache DROP PARTITION (fs_root, old_snapshot_id);
DELETE FROM fs_snapshots WHERE fs_root = ? AND snapshot_id = old_id;
```

Partition drops are nearly instantaneous regardless of data size.

---

## 3. Ingestion from `stats.gz`

### 3.1. New Go Command: `backup-plans ch-import`

```bash
backup-plans ch-import \
    --stats /path/to/stats.gz \
    --ch-dsn "clickhouse://user:pass@host:9000/db" \
    [--snapshot-id 20251210] \
    [--mark-current]
```

**Parameters:**
- `--stats`: Path to the stats.gz file (gzip-compressed WRStat output)
- `--ch-dsn`: ClickHouse connection string
- `--snapshot-id`: Optional; defaults to `YYYYMMDD` from current date
- `--mark-current`: If set, mark this snapshot as current after import

### 3.2. Ingestion Algorithm

```go
func ImportStats(statsPath, chDSN string, snapshotID uint32, markCurrent bool) error {
    // 1. Open stats.gz and create parser
    file, _ := os.Open(statsPath)
    gzReader, _ := pgzip.NewReader(file)
    parser := stats.NewStatsParser(gzReader)
    
    // 2. Connect to ClickHouse
    conn, _ := clickhouse.Open(&clickhouse.Options{Addr: []string{chDSN}})
    
    // 3. Prepare batch inserters
    fileBatch, _ := conn.PrepareBatch(ctx, 
        "INSERT INTO fs_files (fs_root, snapshot_id, dir_path, name, size, mtime, uid, gid)")
    dirBatch, _ := conn.PrepareBatch(ctx,
        "INSERT INTO fs_dirs (fs_root, snapshot_id, dir_path, uid, gid, mtime)")
    
    // 4. Parse and batch insert
    var fsRoot string
    var fileCount, totalSize uint64
    
    summariser := summary.NewSummariser(parser)
    summariser.AddDirectoryOperation(func() summary.Operation {
        return &importOp{fileBatch, dirBatch, &fsRoot, &fileCount, &totalSize}
    })
    
    if err := summariser.Summarise(); err != nil {
        return err
    }
    
    // 5. Flush remaining batches
    fileBatch.Send()
    dirBatch.Send()
    
    // 6. Register snapshot
    conn.Exec(ctx, `
        INSERT INTO fs_snapshots (fs_root, snapshot_id, snapshot_date, file_count, total_size, is_current)
        VALUES (?, ?, today(), ?, ?, ?)
    `, fsRoot, snapshotID, fileCount, totalSize, markCurrent)
    
    return nil
}

type importOp struct {
    fileBatch, dirBatch driver.Batch
    fsRoot              *string
    fileCount, totalSize *uint64
}

func (op *importOp) Add(info *summary.FileInfo) error {
    // Determine fsRoot from first path
    if *op.fsRoot == "" {
        *op.fsRoot = extractRoot(info.Path)
    }
    
    dirPath := string(info.Path.AppendTo(nil))
    name := string(info.Name)
    
    op.fileBatch.Append(
        *op.fsRoot, snapshotID, dirPath, name,
        info.Size, info.MTime, info.UID, info.GID,
    )
    
    *op.fileCount++
    *op.totalSize += uint64(info.Size)
    
    // Flush batch every 100k rows
    if *op.fileCount % 100000 == 0 {
        op.fileBatch.Send()
        op.fileBatch, _ = conn.PrepareBatch(ctx, "INSERT INTO fs_files ...")
    }
    
    return nil
}
```

### 3.3. Performance Targets

| Operation | Target Time | Notes |
|-----------|-------------|-------|
| Parse + insert 1.3B files | < 45 min | Depends on disk I/O and network |
| Materialized view aggregation | Automatic | Happens during insert |
| Register snapshot | < 1 sec | Single row insert |
| Mark as current | < 1 sec | Triggers backend reload |

**Parallelization:** Different filesystem roots can be imported concurrently
since they use separate partitions.

---

## 4. Rule Evaluation and Aggregation in Go

### 4.1. In-Memory Data Structures

The backend maintains several in-memory structures for fast query response:

```go
// Server holds all runtime state
type Server struct {
    // From plan DB
    directoryRules map[string]*DirRules  // path -> claimed dir + rules
    dirs           map[uint64]*db.Directory
    rules          map[uint64]*db.Rule
    
    // Rule matching
    stateMachine   group.StateMachine[db.Rule]
    
    // From ClickHouse (per current snapshot)
    dirSummaries   map[string]*DirSummary  // path -> recursive stats
    
    // Synchronization
    rulesMu        sync.RWMutex
}

// DirRules combines a claimed directory with its rules
type DirRules struct {
    *db.Directory
    Rules map[string]*db.Rule  // match -> rule
}

// DirSummary contains recursive statistics for a directory
type DirSummary struct {
    UID, GID      uint32              // directory owner
    RuleSummaries []RuleSummary       // per-rule aggregates
    Children      map[string]*DirSummary
}

// RuleSummary aggregates files matching a rule
type RuleSummary struct {
    ID     uint64
    Users  []UserStats   // per-UID stats
    Groups []GroupStats  // per-GID stats
}

// UserStats/GroupStats hold counts per user/group
type UserStats struct {
    ID       uint32
    Name     string
    MaxMTime int64
    Files    uint64
    Size     uint64
}
```

### 4.2. Building Aggregates from ClickHouse

At startup (or when a snapshot becomes current), build the in-memory
`dirSummaries` map:

```go
func (s *Server) buildAggregates(ctx context.Context) error {
    s.rulesMu.Lock()
    defer s.rulesMu.Unlock()
    
    // 1. Rebuild state machine from current rules
    s.stateMachine = s.buildStateMachine()
    
    // 2. Query current snapshots for all roots
    snapshots, _ := s.queryCurrentSnapshots(ctx)
    
    // 3. For each root, build aggregates
    for _, snap := range snapshots {
        if err := s.buildRootAggregates(ctx, snap); err != nil {
            return err
        }
    }
    
    return nil
}

func (s *Server) buildRootAggregates(ctx context.Context, snap Snapshot) error {
    // Query fs_dir_stats ordered by dir_path
    rows, _ := s.ch.Query(ctx, `
        SELECT dir_path, uid, gid, file_count, total_size, max_mtime
        FROM fs_dir_stats FINAL
        WHERE fs_root = ? AND snapshot_id = ?
        ORDER BY dir_path
    `, snap.FSRoot, snap.SnapshotID)
    
    // Process in path order, maintaining a stack of ancestors
    var stack []*dirBuilder
    
    for rows.Next() {
        var dirPath string
        var uid, gid uint32
        var fileCount, totalSize uint64
        var maxMTime time.Time
        rows.Scan(&dirPath, &uid, &gid, &fileCount, &totalSize, &maxMTime)
        
        // Pop stack until we find an ancestor
        for len(stack) > 0 && !strings.HasPrefix(dirPath, stack[len(stack)-1].path) {
            finished := stack[len(stack)-1]
            stack = stack[:len(stack)-1]
            s.finishDir(finished)
        }
        
        // Find or create builder for this directory
        builder := s.getOrCreateBuilder(stack, dirPath)
        
        // Determine effective rule for files in this directory
        // Use state machine with directory path
        state := s.stateMachine.GetStateString(dirPath)
        rule := state.GetGroup()
        ruleID := uint64(0)
        if rule != nil {
            ruleID = uint64(rule.ID())
        }
        
        // Add stats to this directory and all ancestors
        builder.addStats(ruleID, uid, gid, fileCount, totalSize, maxMTime)
        for _, ancestor := range stack {
            ancestor.addStats(ruleID, uid, gid, fileCount, totalSize, maxMTime)
        }
    }
    
    // Flush remaining stack
    for len(stack) > 0 {
        s.finishDir(stack[len(stack)-1])
        stack = stack[:len(stack)-1]
    }
    
    return nil
}
```

**Performance:** With ~50M aggregated rows in `fs_dir_stats`, this takes
~2-5 minutes depending on rule count and tree depth.

### 4.3. `/api/tree` Implementation

With in-memory aggregates, `/api/tree` becomes a simple lookup:

```go
func (s *Server) tree(w http.ResponseWriter, r *http.Request) error {
    dir := r.FormValue("dir")
    user := s.getUser(r)
    uid, groups := users.GetIDs(user)
    
    s.rulesMu.RLock()
    defer s.rulesMu.RUnlock()
    
    // 1. Look up pre-computed summary
    summary, ok := s.dirSummaries[dir]
    if !ok {
        return ErrNotFound
    }
    
    // 2. Check authorization
    if !s.isAuthorized(summary, uid, groups) {
        return ErrNotAuthorized
    }
    
    // 3. Build response
    resp := &TreeResponse{
        DirSummary: summary,
        Rules:      s.collectRulesForDir(dir),
        ClaimedBy:  s.getClaimedBy(dir),
        // ... other fields
    }
    
    return json.NewEncoder(w).Encode(resp)
}
```

**Latency:** < 10ms typical (pure memory access + JSON serialization)

### 4.4. Incremental Updates on Rule Changes

When a rule is created, updated, or removed:

```go
func (s *Server) createRule(w http.ResponseWriter, r *http.Request) error {
    dir := r.FormValue("dir")
    rule := parseRule(r)
    
    s.rulesMu.Lock()
    defer s.rulesMu.Unlock()
    
    // 1. Validate and persist to plan DB
    if err := s.rulesDB.CreateDirectoryRule(directory, rule); err != nil {
        return err
    }
    
    // 2. Update in-memory rule maps
    s.directoryRules[dir].Rules[rule.Match] = rule
    s.rules[uint64(rule.ID())] = rule
    
    // 3. Rebuild state machine (fast: milliseconds for 1000s of rules)
    s.stateMachine = s.buildStateMachine()
    
    // 4. Recompute aggregates for affected subtree only
    if err := s.recomputeSubtree(dir); err != nil {
        return err
    }
    
    w.WriteHeader(http.StatusNoContent)
    return nil
}

func (s *Server) recomputeSubtree(dir string) error {
    // Find the filesystem root containing this directory
    fsRoot := s.findFSRoot(dir)
    snap := s.currentSnapshots[fsRoot]
    
    // Query fs_dir_stats only for paths under `dir`
    rows, _ := s.ch.Query(ctx, `
        SELECT dir_path, uid, gid, file_count, total_size, max_mtime
        FROM fs_dir_stats FINAL
        WHERE fs_root = ? AND snapshot_id = ? AND startsWith(dir_path, ?)
        ORDER BY dir_path
    `, fsRoot, snap.SnapshotID, dir)
    
    // Process and update in-memory structures
    // (similar to buildRootAggregates but only for subtree)
    
    // Also update ancestor directories' aggregates
    s.propagateToAncestors(dir)
    
    return nil
}
```

**Latency for rule changes:**
- State machine rebuild: < 100ms for 1000s of rules
- Subtree recomputation: depends on subtree size
  - Small directories: < 100ms
  - Large project roots (100K+ dirs): 1-5 seconds
- Ancestor propagation: < 10ms

### 4.5. State Machine Details

The `wrstat-ui/summary/group.StateMachine` is a byte-by-byte automaton:

```go
// PathGroup defines a rule pattern and its associated data
type PathGroup[T any] struct {
    Path  []byte  // Full pattern, e.g., "/data/project/*.txt"
    Group *T      // Associated rule data
}

// Build state machine from rules
func buildStateMachine(rules []DirRule) group.StateMachine[db.Rule] {
    groups := make([]group.PathGroup[db.Rule], 0, len(rules))
    
    for _, dr := range rules {
        pattern := dr.Directory.Path + dr.Rule.Match
        groups = append(groups, group.PathGroup[db.Rule]{
            Path:  []byte(pattern),
            Group: dr.Rule,
        })
    }
    
    sm, _ := group.NewStatemachine(groups)
    return sm
}

// Evaluate a file path against the state machine
func (sm StateMachine[T]) GetGroup(info *summary.FileInfo) *T {
    // Traverse path byte-by-byte through automaton
    state := sm.getPathState(info.Path)      // directory path
    state = sm.getState(state, info.Name)    // filename
    return sm[state].Group
}
```

**Key properties:**
- O(path-length) evaluation per file
- Handles `*` wildcards via epsilon transitions
- "Most specific wins" is encoded in automaton structure
- Memory: ~10-100 KB for 1000s of rules

---

## 5. Backup Job with ClickHouse

### 5.1. Approach Selection

Two approaches for backup generation:

**A. Stateless ClickHouse Query (Recommended for accuracy)**
- For each BackupIBackup rule, query ClickHouse for matching files
- Handles all edge cases correctly
- May be slower for broad rules

**B. In-Memory Walk (Recommended for speed)**
- Use `fs_rule_dir_cache` to find directories with BackupIBackup files
- Query only those specific directories from `fs_files`
- Faster but requires cache to be up-to-date

### 5.2. Implementation (Hybrid Approach)

```go
func (s *Server) GenerateBackups(ctx context.Context, client *ibackup.MultiClient) error {
    // 1. Find all directories with BackupIBackup rules
    backupDirs := s.findBackupDirectories()
    
    // 2. For each directory, generate file list
    for dir, rules := range backupDirs {
        fofn, err := s.generateFOFN(ctx, dir, rules)
        if err != nil {
            return err
        }
        
        // 3. Submit to ibackup
        dirInfo := s.dirs[dir.ID]
        err = client.Backup(
            dirInfo.Path,
            "plan::" + dirInfo.Path,
            dirInfo.ClaimedBy,
            fofn,
            int(dirInfo.Frequency),
            dirInfo.ReviewDate,
            dirInfo.RemoveDate,
        )
        if err != nil {
            log.Printf("backup failed for %s: %v", dirInfo.Path, err)
        }
    }
    
    return nil
}

func (s *Server) generateFOFN(ctx context.Context, dir *db.Directory, rules []*db.Rule) ([]string, error) {
    // Build WHERE clause from rules
    // Each rule: dir.Path + rule.Match pattern
    
    var conditions []string
    for _, rule := range rules {
        if rule.BackupType != db.BackupIBackup {
            continue
        }
        
        pattern := convertGlobToLike(rule.Match)
        condition := fmt.Sprintf(
            "(startsWith(dir_path, '%s') AND match(name, '%s'))",
            escapeSQLString(dir.Path),
            pattern,
        )
        conditions = append(conditions, condition)
    }
    
    // Query matching files
    query := fmt.Sprintf(`
        SELECT concat(dir_path, name) AS path
        FROM fs_files
        WHERE fs_root = ? AND snapshot_id = ?
          AND (%s)
        ORDER BY path
    `, strings.Join(conditions, " OR "))
    
    rows, _ := s.ch.Query(ctx, query, fsRoot, snapshotID)
    
    var files []string
    for rows.Next() {
        var path string
        rows.Scan(&path)
        
        // Verify this file's effective rule is indeed BackupIBackup
        // (handles override cases)
        if s.effectiveRuleIs(path, db.BackupIBackup) {
            files = append(files, path)
        }
    }
    
    return files, nil
}
```

### 5.3. Performance Optimization

For very large directories, stream directly to ibackup:

```go
func (s *Server) streamBackup(ctx context.Context, dir *db.Directory, rules []*db.Rule, client *ibackup.MultiClient) error {
    // Create streaming FOFN writer
    writer := client.StartBackup(
        "plan::" + dir.Path,
        dir.ClaimedBy,
        dir.Frequency,
        dir.ReviewDate,
        dir.RemoveDate,
    )
    defer writer.Close()
    
    // Stream files directly from ClickHouse
    rows, _ := s.ch.Query(ctx, query, ...)
    
    for rows.Next() {
        var path string
        rows.Scan(&path)
        
        if s.effectiveRuleIs(path, db.BackupIBackup) {
            writer.AddFile(path)
        }
    }
    
    return writer.Commit()
}
```

---

## 6. Go Interface Definitions

### 6.1. ClickHouse Client Interface

```go
// CHClient abstracts ClickHouse operations
type CHClient interface {
    // Query executes a SELECT and returns rows
    Query(ctx context.Context, query string, args ...any) (CHRows, error)
    
    // Exec executes a non-SELECT statement
    Exec(ctx context.Context, query string, args ...any) error
    
    // PrepareBatch starts a batch insert
    PrepareBatch(ctx context.Context, query string) (CHBatch, error)
}

type CHRows interface {
    Next() bool
    Scan(dest ...any) error
    Close() error
}

type CHBatch interface {
    Append(args ...any) error
    Send() error
}
```

### 6.2. Snapshot Manager Interface

```go
// SnapshotManager handles snapshot lifecycle
type SnapshotManager interface {
    // GetCurrent returns the current snapshot for a filesystem root
    GetCurrent(ctx context.Context, fsRoot string) (*Snapshot, error)
    
    // GetAllCurrent returns current snapshots for all roots
    GetAllCurrent(ctx context.Context) ([]Snapshot, error)
    
    // MarkCurrent marks a snapshot as current
    MarkCurrent(ctx context.Context, fsRoot string, snapshotID uint32) error
    
    // DeleteOld deletes snapshots older than the given ID
    DeleteOld(ctx context.Context, fsRoot string, olderThan uint32) error
}

type Snapshot struct {
    FSRoot      string
    SnapshotID  uint32
    SnapshotDate time.Time
    FileCount   uint64
    TotalSize   uint64
}
```

### 6.3. RootDir Interface (Preserved)

The existing `ruletree.RootDir` interface is preserved:

```go
type RootDir interface {
    // Summary returns aggregated stats for a directory
    Summary(path string) (*DirSummary, error)
    
    // GetOwner returns UID/GID for a directory
    GetOwner(path string) (uid, gid uint32, err error)
    
    // IsDirectory checks if a path is a directory
    IsDirectory(path string) bool
    
    // AddRule adds a rule and updates aggregates
    AddRule(dir *db.Directory, rule *db.Rule) error
    
    // RemoveRule removes a rule and updates aggregates
    RemoveRule(dir *db.Directory, rule *db.Rule) error
}
```

The ClickHouse implementation (`CHRootDir`) implements this interface using
ClickHouse as the data source instead of `tree.MemTree`.

---

## 7. Implementation Checklist

### Phase 1: ClickHouse Infrastructure
- [ ] Set up ClickHouse instance with appropriate resources
- [ ] Create schema (tables, views) per section 2
- [ ] Implement `CHClient` wrapper

### Phase 2: Data Ingestion
- [ ] Implement `ch-import` command per section 3
- [ ] Add `SnapshotManager` implementation
- [ ] Test with sample stats.gz files
- [ ] Benchmark ingestion performance (target: < 1hr for 1.3B files)

### Phase 3: Aggregate Building
- [ ] Implement `buildAggregates()` per section 4.2
- [ ] Implement state machine integration
- [ ] Implement `recomputeSubtree()` for incremental updates
- [ ] Benchmark aggregate building (target: < 5 min)

### Phase 4: API Integration
- [ ] Implement `CHRootDir` with same interface as current `RootDir`
- [ ] Update `backend.Server` to use ClickHouse-backed aggregates
- [ ] Verify `/api/tree` meets < 1s latency requirement
- [ ] Verify rule changes reflect in UI within < 1s

### Phase 5: Backup Generation
- [ ] Implement ClickHouse-based backup file enumeration
- [ ] Verify backup job completes in < 1hr (target: < 10 min)
- [ ] Test with ibackup integration

### Phase 6: Testing and Migration
- [ ] Create test fixtures with known stats.gz and rules
- [ ] Verify identical results between old and new implementations
- [ ] Add integration tests for all API endpoints
- [ ] Document migration procedure from current tree DB

### Phase 7: Future Extensions
- [ ] Implement `recursive` flag for `*` vs `**` behavior
- [ ] Implement `priority` flag for non-overridable rules
- [ ] Update state machine to respect new flags

---

## 8. Appendix: Query Examples

### 8.1. Get Current Snapshot for Root

```sql
SELECT fs_root, snapshot_id, snapshot_date, file_count, total_size
FROM fs_snapshots FINAL
WHERE fs_root = '/lustre/scratch123/humgen/'
  AND is_current = 1;
```

### 8.2. Get Directory Stats for Aggregation

```sql
SELECT dir_path, uid, gid, 
       sum(file_count) AS file_count,
       sum(total_size) AS total_size,
       max(max_mtime) AS max_mtime
FROM fs_dir_stats FINAL
WHERE fs_root = '/lustre/scratch123/humgen/'
  AND snapshot_id = 20251210
GROUP BY dir_path, uid, gid
ORDER BY dir_path;
```

### 8.3. Check if Path is Directory

```sql
SELECT 1
FROM fs_dirs
WHERE fs_root = '/lustre/scratch123/humgen/'
  AND snapshot_id = 20251210
  AND dir_path = '/lustre/scratch123/humgen/projects/my-project/'
LIMIT 1;
```

### 8.4. Get Directory Owner

```sql
SELECT uid, gid
FROM fs_dirs
WHERE fs_root = '/lustre/scratch123/humgen/'
  AND snapshot_id = 20251210
  AND dir_path = '/lustre/scratch123/humgen/projects/my-project/';
```

### 8.5. Get Files Matching a Rule Pattern

```sql
SELECT concat(dir_path, name) AS path
FROM fs_files
WHERE fs_root = '/lustre/scratch123/humgen/'
  AND snapshot_id = 20251210
  AND startsWith(dir_path, '/lustre/scratch123/humgen/projects/my-project/')
  AND match(name, '^.*\\.txt$')  -- converted from *.txt
ORDER BY path;
```

### 8.6. Delete Old Snapshot

```sql
-- Drop partitions (instantaneous)
ALTER TABLE fs_files DROP PARTITION ('/lustre/scratch123/humgen/', 20251201);
ALTER TABLE fs_dirs DROP PARTITION ('/lustre/scratch123/humgen/', 20251201);
ALTER TABLE fs_dir_stats DROP PARTITION ('/lustre/scratch123/humgen/', 20251201);
ALTER TABLE fs_rule_dir_cache DROP PARTITION ('/lustre/scratch123/humgen/', 20251201);

-- Remove snapshot record
ALTER TABLE fs_snapshots DELETE 
WHERE fs_root = '/lustre/scratch123/humgen/' AND snapshot_id = 20251201;
```
