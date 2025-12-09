# ClickHouse Implementation Specs for Backup Plans

## 1. Overview
This document specifies the architecture for replacing the custom `treegen` database in the `backup-plans` application with ClickHouse. The goal is to support 1.3 billion+ files, allow flexible rule matching (globs, exclusions), and provide <1s response times for user interactions.

## 2. Schema Design

### 2.1. Table: `files`
This is the main table containing the daily snapshot of the filesystem.

```sql
CREATE TABLE files (
    path String,
    name String,      -- Extracted from path for easier globbing (optional, but useful)
    size UInt64,
    mtime UInt64,
    uid UInt32,
    gid UInt32,
    -- Partitioning by date allows easy cleanup of old snapshots
    snapshot_date Date DEFAULT today()
)
ENGINE = MergeTree()
PARTITION BY snapshot_date
ORDER BY (path) 
SETTINGS index_granularity = 8192;
```

*   **Ordering Key (`path`)**: Critical. It allows queries starting with a directory prefix (e.g., `WHERE path LIKE '/nfs/users/sb10/%'`) to seek directly to the data range, skipping 99.9% of the table.

### 2.2. Table: `rule_stats`
A cache table to store the aggregate impact of each rule. This avoids re-scanning the `files` table for global reports.

```sql
CREATE TABLE rule_stats (
    rule_id UInt64,
    snapshot_date Date,
    file_count UInt64,
    total_size UInt64,
    last_updated DateTime DEFAULT now()
)
ENGINE = ReplacingMergeTree(last_updated)
ORDER BY (rule_id, snapshot_date);
```

## 3. Ingestion Process (`cmd/db.go` replacement)

The daily ingestion process must be rewritten to stream data into ClickHouse.

1.  **Parse**: Read `stats.gz` using the existing `stats` parser.
2.  **Batch**: Accumulate rows (e.g., 100,000 at a time) in memory.
3.  **Insert**: Use the ClickHouse Go client to bulk insert into `files`.
    *   *Performance Target*: 1.3B rows should ingest in < 30-45 mins on standard hardware using parallel inserts (4-8 threads).
4.  **Cleanup**: `ALTER TABLE files DROP PARTITION <old_date>` to remove data older than X days.

## 4. Query Logic & Application Behavior

### 4.1. User Adds/Edits a Rule
**Scenario**: User adds a rule `*.bam` to directory `/nfs/project_A/`.

1.  **Calculate Scope**: The application identifies the directory scope: `/nfs/project_A/`.
2.  **Query ClickHouse**:
    ```sql
    SELECT count(), sum(size)
    FROM files
    WHERE path LIKE '/nfs/project_A/%'  -- Uses Primary Key Index (Fast)
      AND name LIKE '%.bam'             -- Filter applied only to the range
      AND snapshot_date = today()
    ```
3.  **Update Cache**: Store the result in `rule_stats` for this `rule_id`.
4.  **Response**: Return the numbers to the user. Latency: <1s (due to index seek).

### 4.2. Global Frontend Stats (The "Tree View")
**Scenario**: User views the root or a high-level directory and wants to see "Total Backed Up".

1.  **Fetch Rules**: Load all active rules from the SQL Plan DB.
2.  **Fetch Stats**: `SELECT * FROM rule_stats WHERE snapshot_date = today()`.
3.  **Compute (In-Memory)**:
    *   The application (Go) maps rules to the file tree structure.
    *   **Inclusion**: Add `total_size` of matching rules.
    *   **Exclusion/Overlap**:
        *   If Rule A is `/data` (Include) and Rule B is `/data/tmp` (Exclude).
        *   Total = `Stats(Rule A) - Stats(Rule B)`.
    *   *Note*: This arithmetic is valid because we calculated Rule A's stats based on *all* files in `/data`, and Rule B's based on *all* files in `/data/tmp`.
    *   For overlapping "Include" rules (e.g. `*` and `*.bam` in same dir), the application must decide precedence or use a slightly more complex query in step 4.1 to exclude previously matched files if strict disjoint counts are needed.

### 4.3. The Backup Process (Iterating for Backup Client)
**Scenario**: Generate the list of files to backup.

Instead of iterating 1.3B files in Go, we stream specific file lists.

1.  **Group Rules**: Group rules by their top-level disjoint directories.
2.  **Stream**: For each group, generate a SQL query.
    *   *Example*: Rule A (`/data/*`), Rule B (`/data/tmp/*` - Exclude).
    ```sql
    SELECT path
    FROM files
    WHERE path LIKE '/data/%'
      AND path NOT LIKE '/data/tmp/%'
      AND snapshot_date = today()
    FORMAT RowBinary
    ```
3.  **Parallelism**: Run these queries in parallel for different top-level directories.
4.  **Output**: Stream the resulting paths directly to the backup system/file list.

## 5. Handling Complex Globs & "Special" Rules

### 5.1. Glob Support (`*` vs `**`)
ClickHouse supports SQL `LIKE` (simple globs) and `match()` (Re2 regex).
*   **`*` (Current folder only)**:
    *   SQL: `path LIKE '/dir/%' AND countMatches(path, '/') = <depth>`
    *   *Alternative*: Store `parent_dir` column in ClickHouse to make this trivial: `WHERE parent_dir = '/dir'`.
*   **`**` (Recursive)**:
    *   SQL: `path LIKE '/dir/%'`

### 5.2. ".gitignore" Style
Translate the patterns into SQL conditions.
*   Rule: `src/**/*.o`
*   SQL: `path LIKE '/src/%' AND path LIKE '%.o'`

## 6. Implementation Steps

1.  **Infrastructure**: Spin up ClickHouse instance.
2.  **Ingestor**: Create `cmd/ingest_clickhouse.go` to replace `cmd/db.go`.
3.  **Backend Adapter**: Create a new implementation of the `backend` interfaces that speaks SQL instead of reading `tree.db`.
    *   Replace `ruletree` package usage with SQL queries.
4.  **Frontend/API**: Update endpoints to trigger `rule_stats` updates on rule changes.
