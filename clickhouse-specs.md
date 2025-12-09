# ClickHouse Implementation Specs for Backup Plans

## 1. Overview
This document specifies the architecture for replacing the custom `treegen` database in the `backup-plans` application with ClickHouse. The goal is to support 1.3 billion+ files, allow flexible rule matching (globs, exclusions), and provide <1s response times for user interactions, including the "Tree View" with user/group filtering.

## 2. Schema Design

### 2.1. Table: `files`
The raw inventory of files.

```sql
CREATE TABLE files (
    path String,
    name String,      -- Extracted from path
    size UInt64,
    mtime UInt64,
    uid UInt32,
    gid UInt32,
    snapshot_date Date DEFAULT today()
)
ENGINE = MergeTree()
PARTITION BY snapshot_date
ORDER BY (path) 
SETTINGS index_granularity = 8192;
```

### 2.2. Materialized View: `dir_stats_mv` & Table: `dir_stats`
To support fast "Tree View" browsing without scanning 1.3B rows, we pre-aggregate stats at the directory level.

```sql
CREATE TABLE dir_stats (
    dir_path String, -- The directory containing the files
    uid UInt32,
    gid UInt32,
    file_count UInt64,
    total_size UInt64,
    max_mtime UInt64,
    snapshot_date Date
)
ENGINE = SummingMergeTree(file_count, total_size, max_mtime)
PARTITION BY snapshot_date
ORDER BY (dir_path, uid, gid);

CREATE MATERIALIZED VIEW dir_stats_mv TO dir_stats AS
SELECT
    cutToFirstSignificantSubdomain(path) as dir_path, -- Logic to extract parent dir
    uid,
    gid,
    count() as file_count,
    sum(size) as total_size,
    max(mtime) as max_mtime,
    snapshot_date
FROM files
GROUP BY dir_path, uid, gid, snapshot_date;
```
*Note*: The `cutToFirstSignificantSubdomain` is a placeholder. In practice, use `substring(path, 1, lastIndex(path, '/'))` or similar to get the parent directory.

### 2.3. Table: `rule_cache`
Stores the "Net Stats" for defined rules to allow instant retrieval of root-level numbers.

```sql
CREATE TABLE rule_cache (
    rule_id UInt64,
    dir_path String, -- The directory the rule applies to
    uid UInt32,
    gid UInt32,
    file_count UInt64,
    total_size UInt64,
    snapshot_date Date
)
ENGINE = SummingMergeTree()
PARTITION BY snapshot_date
ORDER BY (rule_id, dir_path, uid, gid);
```

## 3. Ingestion Process

1.  **Load Rules**: Fetch all current rules from the SQL Plan DB.
2.  **Stream Ingestion**: Read `stats.gz`.
    *   For each file, determine which rule(s) apply.
    *   *Optimization*: We can't easily tag `rule_id` in `files` if rules overlap complexly.
    *   *Alternative*: Ingest raw `files`. Let `dir_stats` populate automatically.
    *   Then, populate `rule_cache` by running aggregation queries on `dir_stats` for each rule's scope.
        *   `INSERT INTO rule_cache SELECT rule_id, dir_path, uid, gid, sum(file_count), ... FROM dir_stats WHERE dir_path LIKE '/rule/path/%' ...`
        *   Handle exclusions (child rules) by subtracting their stats.

## 4. Query Logic & Application Behavior

### 4.1. The "Tree View" (Browsing)
**Requirement**: View `/a/b`. Show stats for `/a/b` (recursive) and its children `/a/b/c`, `/a/b/d`. Support filtering by User/Group.

1.  **Fetch Stats**:
    *   Query `dir_stats` for the requested directory and its subdirectories.
    *   `SELECT dir_path, uid, gid, sum(file_count), sum(total_size) FROM dir_stats WHERE dir_path LIKE '/a/b/%' GROUP BY dir_path, uid, gid`
    *   This returns stats for every subdirectory.
2.  **Aggregate in Go**:
    *   The application receives the list of all subdirectories.
    *   It builds the tree structure in memory.
    *   It applies the "Rule Logic" (which rule matches which directory).
    *   It aggregates `uid`/`gid` stats up the tree to populate `RuleSummaries` for `/a/b` and its immediate children.
    *   *Performance*: `dir_stats` is ~10-100x smaller than `files`. Scanning a subtree is fast.

### 4.2. Adding a New Rule (Instant Feedback)
**Scenario**: User adds Rule C at `/a/b/new`.

1.  **Calculate Rule C Stats**:
    *   Query `dir_stats`: `SELECT uid, gid, sum(file_count), sum(total_size) FROM dir_stats WHERE dir_path LIKE '/a/b/new/%' GROUP BY uid, gid`.
    *   This is extremely fast (indexed seek).
2.  **Update Frontend**:
    *   The application calculates the "Delta" (Stats of Rule C).
    *   It subtracts this Delta from the stats of the parent Rule A (cached or fetched).
    *   It returns the updated view to the user.
    *   *Note*: This handles the "don't let this special rule be overridden" requirement by explicitly calculating the new rule's scope and removing it from the parent's scope in the display logic.

### 4.3. Filtering
The frontend filters by User/Group client-side.
*   **Requirement**: The backend must send `RuleSummaries` containing `Users: Stats[]` and `Groups: Stats[]`.
*   **Implementation**: The `dir_stats` table preserves `uid`/`gid`. The aggregation in Step 4.1 preserves this granularity. The backend constructs the `Stats[]` arrays from this data.

## 5. Handling "Net" Stats (Exclusions)
The user wants to see how many files *actually* match a rule (excluding those matched by child rules).

*   **Logic**:
    *   Rule A matches `/a`. Rule B matches `/a/b`.
    *   Stats(Rule A) = Total(`/a`) - Total(`/a/b`).
*   **Implementation**:
    *   The application fetches stats for `/a` (recursive) and `/a/b` (recursive) from `dir_stats`.
    *   It performs the subtraction.
    *   This logic is decoupled from the database, allowing flexibility.

## 6. Backup Process
**Scenario**: Generate file list for backup.

1.  **Disjoint Queries**:
    *   For Rule A (`/a`, exclude `/a/b`):
    *   `SELECT path FROM files WHERE path LIKE '/a/%' AND path NOT LIKE '/a/b/%'`
2.  **Stream**:
    *   Stream results directly to the backup client.

## 7. Implementation Plan

1.  **ClickHouse Setup**: Deploy with `files` and `dir_stats` (MV).
2.  **Ingestor**: Go program to parse `stats.gz` and bulk insert to `files`.
3.  **Backend**:
    *   Implement `backend.Summariser` interface using ClickHouse queries.
    *   `Summary(path)`:
        *   Query `dir_stats` for `path + "%"`.
        *   Load Rules.
        *   Map directories to rules.
        *   Aggregate stats (summing `dir_stats` rows) per rule/user/group.
        *   Construct `DirSummary` object.
4.  **Optimization**:
    *   If `dir_stats` is still too large for root queries, implement `rule_cache` (Step 2.3) to store pre-calculated root sums.
