# Watch FOFNs Integration Specification

## Overview

The backup-plans system currently backs up files matching plan rules by sending
ibackup sets to external ibackup servers. ibackup v1.16.0 introduced a
"watchfofns" mode where an `ibackup watchfofns` process monitors a directory
tree for null-terminated FOFN files and their associated config.yml files, then
backs up the listed files autonomously.

This feature adds watchfofns support to backup-plans. When a server entry in the
config YAML has a `fofndir` property, `backup-plans backup` will - in addition
to (or instead of) sending sets to the ibackup server API - create
subdirectories under fofndir, each containing a null-terminated FOFN file and a
`config.yml`. `backup-plans server` will parse status files produced by `ibackup
watchfofns` so it can report backup status for each claimed directory via
fofn-based backups. Both mechanisms (server API and fofn files) can operate in
parallel during a transition period.

## Architecture

### Dependency upgrade

The `github.com/wtsi-hgi/ibackup` dependency must be upgraded from v1.13.0 to
v1.16.0 to gain access to the `fofn` package.

### New files

| File | Responsibility | Stories |
|---|---|---|
| `ibackup/fofndir.go` | Streaming write of fofn subdirectories (FOFN file + config.yml), filesystem-safe naming, frequency gating | A1, A2, A3, A4 |
| `ibackup/fofndir_test.go` | Tests for fofndir.go | A1, A2, A3, A4 |
| `ibackup/fofnstatus.go` | Parse status files, map to SetBackupActivity, cache | B1, B2 |
| `ibackup/fofnstatus_test.go` | Tests for fofnstatus.go | B1, B2 |

### Changes to existing files

| File | Change | Stories |
|---|---|---|
| `go.mod` | Upgrade ibackup to v1.16.0 | all |
| `config/config.go` | Add FofnDir field to ServerDetails in yamlConfig, expose per-path fofndir via new getter, build fofn status caches | B2, C1, C3 |
| `config/config_test.go` | Test FofnDir config parsing | C1, C3 |
| `ibackup/ibackup.go` | Add FofnDir to ServerDetails and Config; extend MultiClient to expose fofndir per path; extend SetBackupActivity with optional fofn status counts; extend MultiCache to also cache fofn status; skip connect() for fofndir-only servers in New()/createServers() | A1, B1, B2, C2 |
| `ibackup/ibackup_test.go` | Tests for extended types | A1, B1 |
| `backups/backups.go` | Call fofndir writer for directories whose path matches a server with fofndir configured | C1 |
| `backups/backups_test.go` | Test fofn file creation alongside ibackup sets | C1 |
| `backend/report.go` | Populate fofn-based backup status in summary using fofn status cache | D1 |
| `backend/report_test.go` | Test fofn status in summary (if exists; otherwise covered by integration) | D1 |
| `frontend/src/types.ts` | Extend BackupStatus with optional fofn status count fields | D2 |
| `frontend/src/report.ts` | Display fofn status counts in report table | D2 |
| `cmd/backup.go` | Wire fofn writer into backup flow | C1 |
| `main_test.go` | Integration test for fofn file creation and status | E1 |

### Key types and interfaces

#### Extended ServerDetails (ibackup package)

```go
// ServerDetails contains the connection details for a particular
// ibackup server.
type ServerDetails struct {
    Addr, Cert, Token, Username string
    FofnDir                     string
}
```

The `FofnDir` field is optional. When set, it specifies the directory where fofn
subdirectories will be created for the corresponding server. The `Addr`, `Cert`,
`Token`, and `Username` fields remain optional - if omitted, no API-based
ibackup sets are created for that server entry.

#### Extended SetBackupActivity (ibackup package)

```go
// SetBackupActivity holds info about backup activity retrieved
// from an ibackup server or fofn status file.
type SetBackupActivity struct {
    LastSuccess time.Time
    Name        string
    Requester   string
    Failures    uint64
    Uploaded    int
    Replaced    int
    Unmodified  int
    Missing     int
    Frozen      int
    Orphaned    int
    Warning     int
    Hardlink    int
}
```

The new count fields (Uploaded through Hardlink) are populated only from fofn
status files. They default to zero and are omitted from JSON when zero (using
`json:",omitempty"` tags) so that the existing frontend behaviour is unaffected
until it is updated to display them. `Failures` continues to hold the failure
count from whichever source (ibackup API `Failed` field or fofn status `Failed`
count).

#### FofnDirWriter (ibackup package, fofndir.go)

```go
// FofnDirWriter writes null-terminated FOFN files and
// config.yml files into subdirectories under a base fofn
// directory.
type FofnDirWriter struct {
    baseDir string
}

// NewFofnDirWriter returns a new FofnDirWriter that creates
// subdirectories under baseDir.
func NewFofnDirWriter(baseDir string) *FofnDirWriter

// SafeName converts a set name (e.g. "plan::/lustre/scratch/a/")
// to a filesystem-safe directory name by stripping the "plan::"
// prefix and replacing '/' with the fullwidth solidus character
// (U+FF0F).
func SafeName(setName string) string

// Write creates (or updates) the fofn subdirectory for the
// given set. It writes the null-terminated FOFN file and the
// config.yml. If the FOFN file already exists and its
// modification time is within the frequency window (same logic
// as ibackup set frequency check), the write is skipped and
// (false, nil) is returned. On successful write, (true, nil)
// is returned.
//
// The files iterator yields absolute file paths. Write streams
// them directly to the FOFN file without accumulating them in
// memory, so arbitrarily large file lists are supported.
//
// frequency is in days. If frequency is 0, the set is frozen
// and freeze will be set to true in config.yml. The fofn is
// written on initial creation but skipped if it already exists.
//
// The caller is responsible for populating the metadata map
// with requestor, review, and remove entries before calling
// Write.
func (f *FofnDirWriter) Write(
    setName string,
    transformer string,
    files iter.Seq[string],
    frequency int,
    metadata map[string]string,
) (bool, error)

// UpdateConfig rewrites only the config.yml in the fofn
// subdirectory for the given set name, without touching the
// FOFN file. This is used when a claimed directory is
// transferred to another user.
func (f *FofnDirWriter) UpdateConfig(
    setName string,
    transformer string,
    freeze bool,
    metadata map[string]string,
) error
```

#### FofnStatusReader (ibackup package, fofnstatus.go)

```go
// FofnStatusReader reads status files from fofn subdirectories.
type FofnStatusReader struct {
    baseDir string
}

// NewFofnStatusReader returns a reader that looks for status
// files under baseDir.
func NewFofnStatusReader(baseDir string) *FofnStatusReader

// GetBackupActivity reads the status file for the given set
// name and returns a SetBackupActivity. If the status file
// does not exist, it returns nil and no error (the backup
// has not yet been processed). The LastSuccess is set to the
// status file's modification time when Failed == 0.
func (f *FofnStatusReader) GetBackupActivity(
    setName string,
) (*SetBackupActivity, error)
```

#### Extended MultiClient (ibackup package)

```go
// GetFofnDir returns the fofn directory configured for the
// server matching the given path, or "" if none is configured.
func (m *MultiClient) GetFofnDir(path string) string

// GetTransformer returns the transformer string configured
// for the server matching the given path, or "" if none
// matches.
func (m *MultiClient) GetTransformer(path string) string
```

#### Extended MultiCache (ibackup package)

```go
// GetFofnBackupActivity retrieves fofn-based backup status
// for the given path and set name. Returns nil if no fofndir
// is configured for the path or no status file exists.
func (m *MultiCache) GetFofnBackupActivity(
    path, setName string,
) (*SetBackupActivity, error)
```

#### Config extensions (config package)

No new public types needed. New getter method:

```go
// GetFofnDirs returns a map of path-regexp to fofndir for
// all servers that have a fofndir configured.
func (c *Config) GetFofnDirs() map[string]string
```

### Fofndir-only server handling

When a server entry has a `FofnDir` but no `Addr` (empty string), `New()` /
`createServers()` must skip the `connect()` call for that entry. A nil
`serverClient` is stored for the server so that `createClients()` can still
build the path-to-server routing, but `MultiClient.Backup()` must not attempt to
call ibackup API methods on a nil client. The `MultiClient.Backup()` method must
check whether the client has a valid connection before making API calls and skip
API operations for fofndir-only servers.

Similarly, `NewMultiCache()` must handle nil clients when creating `Cache`
instances - a fofndir-only server entry gets a cache with a nil client (which
will never be queried for API-based status).

This allows config files to specify:

- Only server details (no fofndir) - API-only backup.
- Only fofndir (no server details) - fofn-only backup.
- Both - parallel backup during transition.

### Filesystem-safe naming

Set names like `plan::/lustre/scratch123/humgen/a/b/` are converted to
filesystem-safe directory names by:

1. Stripping the `plan::` prefix.
2. Replacing every `/` (U+002F) with fullwidth solidus `\uff0f` (U+FF0F).

Example: `plan::/lustre/scratch123/humgen/a/b/` becomes
`\uff0flustre\uff0fscratch123\uff0fhumgen\uff0fa\uff0fb\uff0f`.

The `SafeName` function implements this transformation.

### FOFN file format

The FOFN file is a list of absolute file paths separated by null bytes (0x00).
No trailing newline. Example for two files:

```text
/lustre/scratch123/humgen/a/b/1.jpg\x00/lustre/scratch123/humgen/a/b/2.jpg\x00
```

The file is named `fofn` inside the subdirectory.

### Directory layout

Given a fofndir of `/watch/` and a set name of
`plan::/lustre/scratch123/humgen/a/b/`:

```text
/watch/
    <safename>/
        fofn              # null-terminated file list
        config.yml        # transformer, freeze, metadata
        status            # created by ibackup watchfofns
```

### Frequency gating

The frequency check reuses the same logic as the existing ibackup set frequency
check in `ibackup.updateSet()`:

- If the FOFN file does not exist, always write.
- If `frequency == 0` (frozen), never rewrite after initial creation.
- Otherwise, if the FOFN file's modification time plus `(frequency-1) * 24h +
  12h` is after `time.Now()`, skip the write.

### Error handling policy

- **Missing fofndir base directory:** Return an error from
  `FofnDirWriter.Write`. The backup command should log and continue to the next
  directory.
- **Permission errors writing fofn files:** Return an error. The backup command
  logs and continues.
- **Missing status file:** Return nil activity (not an error). The directory has
  not been processed yet.
- **Malformed status file:** Return the parse error. The server should log and
  return a nil activity to the frontend.
- **ibackup dependency upgrade errors:** Build errors from API changes in
  v1.16.0 must be resolved.

---

## Section A: FOFN Directory Writing

### A1: Filesystem-safe set name conversion

As a developer, I want to convert set names to filesystem-safe directory names,
so that each claimed directory gets a unique subdirectory under fofndir.

The `SafeName` function strips the `plan::` prefix and replaces every `/` with
the fullwidth solidus character (U+FF0F).

**Package:** `ibackup/`
**File:** `ibackup/fofndir.go`
**Test file:** `ibackup/fofndir_test.go`

```go
func SafeName(setName string) string
```

**Acceptance tests:**

1. Given the set name `plan::/lustre/scratch123/humgen/a/b/`, when SafeName is
   called, then it returns
   `"\uff0flustre\uff0fscratch123\uff0fhumgen\uff0fa\uff0fb\uff0f"`.

2. Given the set name `plan::/a/`, when SafeName is called, then it returns
   `"\uff0fa\uff0f"`.

3. Given the set name `plan::/`, when SafeName is called, then it returns
   `"\uff0f"`.

4. Given a set name without the `plan::` prefix, e.g. `"/lustre/scratch/"`, when
   SafeName is called, then it returns `"\uff0flustre\uff0fscratch\uff0f"` (the
   function only strips `plan::` if present).

5. Given an empty string, when SafeName is called, then it returns `""`.

### A2: Writing FOFN files and config

As a backup operator, I want the backup command to create fofn subdirectories
with null-terminated FOFN files and config.yml, so that `ibackup watchfofns` can
discover and back up the files.

`FofnDirWriter.Write` creates the subdirectory (using `SafeName` for the
directory name), streams paths from the `iter.Seq[string]` directly to the FOFN
file (one null-terminated path at a time, never accumulating all paths in
memory), and writes config.yml using `fofn.WriteConfig` from the ibackup
library.

The metadata map must include:
- `requestor=<username>` (the claiming user)
- `review=<date>` (review date as YYYY-MM-DD)
- `remove=<date>` (remove date as YYYY-MM-DD)

When `frequency == 0`, `freeze` is set to `true` in the config.

**Package:** `ibackup/`
**File:** `ibackup/fofndir.go`
**Test file:** `ibackup/fofndir_test.go`

**Acceptance tests:**

1. Given a base directory and a set of files, when Write is called with
   transformer "humgen", frequency 7, and metadata `{"requestor": "userA",
   "review": "2027-01-01", "remove": "2027-06-01"}`, then:
   - A subdirectory named SafeName(setName) exists under baseDir.
   - A file named `fofn` exists in that subdirectory.
   - The fofn file contains each file path separated by null bytes (0x00).
   - A file named `config.yml` exists in that subdirectory.
   - The config.yml, parsed with `fofn.ReadConfig`, has Transformer == "humgen",
     Freeze == false, and Metadata containing "requestor"="userA",
     "review"="2027-01-01", "remove"="2027-06-01".
   - Write returns (true, nil).

2. Given frequency == 0, when Write is called, then the config.yml has Freeze ==
   true.

3. Given an iterator that yields zero paths, when Write is called, then Write
   returns (false, nil) and no subdirectory is created (same as existing
   ibackup.Backup behaviour for empty files).

4. Given a single file "/a/b/c.txt", when Write is called, then the fofn file
   contains exactly `/a/b/c.txt\x00` (the 10-byte path plus a trailing null
   byte).

5. (**Memory test**) Given an iterator that yields 1,000,000 paths of 100 bytes
   each, when Write is called, then `runtime.MemStats.HeapInuse` does not
   increase by more than 10 MB above the baseline measured before the call
   (proving the implementation streams rather than accumulating).

### A3: Frequency gating for FOFN writes

As a backup operator, I want the FOFN file to only be rewritten when enough time
has passed since the last write, so that the system does not overwhelm the
watchfofns process.

The frequency check mirrors `ibackup.updateSet()`: if the FOFN file's mtime plus
`(frequency-1)*24h + 12h` is after `time.Now()`, the write is skipped and
(false, nil) is returned.

For frozen sets (frequency == 0), if the FOFN file already exists, the write is
skipped and (false, nil) is returned; the set is already frozen and should not
be updated.

**Package:** `ibackup/`
**File:** `ibackup/fofndir.go`
**Test file:** `ibackup/fofndir_test.go`

**Acceptance tests:**

1. Given no existing FOFN file, when Write is called with frequency 7, then the
   file is written and (true, nil) is returned.

2. Given an existing FOFN file whose mtime is 1 hour ago, when Write is called
   with frequency 1, then the write is skipped and (false, nil) is returned
   (because 0*24h + 12h = 12h > 1h).

3. Given an existing FOFN file whose mtime is 13 hours ago, when Write is called
   with frequency 1, then the file is rewritten and (true, nil) is returned
   (because 12h < 13h).

4. Given an existing FOFN file whose mtime is 36 hours ago (1.5 days), when
   Write is called with frequency 2, then the write is skipped and (false, nil)
   is returned (because 1*24h + 12h = 36h is not strictly before 36h; need to be
   after the threshold).

5. Given an existing FOFN file whose mtime is 37 hours ago, when Write is called
   with frequency 2, then the file is rewritten and (true, nil) is returned.

6. Given an existing FOFN file and frequency 0 (frozen), when Write is called,
   then the write is skipped and (false, nil) is returned.

7. Given no existing FOFN file and frequency 0 (frozen), when Write is called,
   then the file is written and (true, nil) is returned.

### A4: Config-only update for claim transfer

As a backup operator, I want to update only the config.yml when a directory
claim is transferred to another user, so that the requestor metadata stays
current without rewriting the FOFN.

**Package:** `ibackup/`
**File:** `ibackup/fofndir.go`
**Test file:** `ibackup/fofndir_test.go`

```go
func (f *FofnDirWriter) UpdateConfig(
    setName string,
    transformer string,
    freeze bool,
    metadata map[string]string,
) error
```

**Acceptance tests:**

1. Given an existing fofn subdirectory with a config.yml containing
   requestor=userA, when UpdateConfig is called with metadata containing
   requestor=userB, then the config.yml is rewritten with requestor=userB and
   the FOFN file is not modified.

2. Given a non-existent fofn subdirectory, when UpdateConfig is called, then an
   error is returned (the subdirectory must exist first).

---

## Section B: FOFN Status Reading

### B1: Parsing fofn status files

As a server operator, I want to read status files produced by `ibackup
watchfofns`, so that I can report on the backup status of each claimed
directory.

`FofnStatusReader.GetBackupActivity` looks for a `status` file (which may be a
symlink) in the subdirectory named by `SafeName(setName)` under the baseDir. It
calls `fofn.ParseStatus` from the ibackup library and maps the result to a
`SetBackupActivity`.

Mapping rules:
- `LastSuccess` = status file mtime when `Failed == 0`. When `Failed > 0`,
  `LastSuccess` is zero.
- `Failures` = `StatusCounts.Failed`.
- `Uploaded` = `StatusCounts.Uploaded`.
- `Replaced` = `StatusCounts.Replaced`.
- `Unmodified` = `StatusCounts.Unmodified`.
- `Missing` = `StatusCounts.Missing`.
- `Frozen` = `StatusCounts.Frozen`.
- `Orphaned` = `StatusCounts.Orphaned`.
- `Warning` = `StatusCounts.Warning`.
- `Hardlink` = `StatusCounts.Hardlink`.

**Package:** `ibackup/`
**File:** `ibackup/fofnstatus.go`
**Test file:** `ibackup/fofnstatus_test.go`

**Acceptance tests:**

1. Given a status file with content:
   ```text
   /local/a\t/remote/a\tuploaded\t
   /local/b\t/remote/b\tunmodified\t
   SUMMARY\tuploaded=1\treplaced=0\tunmodified=1\tmissing=0\tfailed=0\tfrozen=0\torphaned=0\twarning=0\thardlink=0\tnot_processed=0
   ```
   When GetBackupActivity is called, then the returned SetBackupActivity has:
   - Failures == 0
   - Uploaded == 1
   - Unmodified == 1
   - LastSuccess equal to the status file's mtime (non-zero)
   - All other count fields == 0.

2. Given a status file with failed=2, when GetBackupActivity is called, then
   Failures == 2, LastSuccess is zero time, and the other count fields reflect
   the status file.

3. Given no status file exists for the set name, when GetBackupActivity is
   called, then nil is returned with no error.

4. Given a status file with all zeros (everything not_processed), when
   GetBackupActivity is called, then LastSuccess equals the file mtime
   (Failed==0 means success), Failures == 0, and all other counts are 0 except
   NotProcessed (which is not mapped to SetBackupActivity but is ignored).

### B2: Caching fofn status

As a server operator, I want fofn status to be cached and periodically
refreshed, so that the server does not read status files on every request.

The fofn status cache reuses the same `MultiCache` pattern. Each server with a
fofndir gets a `FofnStatusReader`. The `MultiCache` gains a
`GetFofnBackupActivity` method that looks up the correct reader by path regex,
calls `GetBackupActivity`, and caches the result. The cache is refreshed on the
same schedule as the ibackup API cache.

**Package:** `ibackup/`
**File:** `ibackup/fofnstatus.go` and `ibackup/ibackup.go`
**Test file:** `ibackup/fofnstatus_test.go`

**Acceptance tests:**

1. Given a MultiCache with a fofndir configured for `^/lustre/`, and a status
   file exists for set name `plan::/lustre/scratch/a/`, when
   GetFofnBackupActivity is called with path "/lustre/scratch/a/" and the set
   name, then the correct SetBackupActivity is returned.

2. Given the same setup, when GetFofnBackupActivity is called twice, then the
   second call returns the cached result without re-reading the status file
   (verified by checking that the file is not re-opened, or by modifying the
   file between calls and seeing the old result).

3. Given no fofndir is configured for the path, when GetFofnBackupActivity is
   called, then nil is returned with no error.

---

## Section C: Backup Command Integration

### C1: Creating fofn files during backup

As a backup operator, I want `backup-plans backup` to create fofn subdirectories
for servers that have fofndir configured, so that `ibackup watchfofns` processes
can discover and back up files.

The `backups.Backup` function is extended to accept fofndir configuration. For
each claimed directory with BackupIBackup rules, if the corresponding server has
a fofndir, a fofn subdirectory is created (or updated subject to frequency
gating). If the server also has API connection details, the existing ibackup set
is also created.

The set name used for fofn directories is the same as the existing ibackup set
name: `"plan::" + dir.Path`.

The metadata map for the config.yml includes:
- `requestor` = directory's ClaimedBy
- `review` = review date formatted as YYYY-MM-DD
- `remove` = remove date formatted as YYYY-MM-DD

The transformer is looked up from the config's PathToServer mapping for the
directory path.

**Package:** `backups/`
**File:** `backups/backups.go`
**Test file:** `backups/backups_test.go`

Also:
**Package:** `config/`
**File:** `config/config.go`
**Test file:** `config/config_test.go`

Also:
**Package:** `ibackup/`
**File:** `ibackup/ibackup.go`

**Acceptance tests:**

1. Given a config with a server that has both API details and a fofndir, and a
   plan DB with a claimed directory with BackupIBackup rules, when Backup is
   called, then:
   - An ibackup set is created via the API (existing behaviour).
   - A fofn subdirectory is created under fofndir with the correct FOFN file and
     config.yml.
   - The SetInfo returned includes the backup set name.

2. Given a config with a server that has only fofndir (no API details), when
   Backup is called, then:
   - No ibackup API call is made (no error from missing server connection).
   - A fofn subdirectory is created under fofndir.

3. Given a config with a server that has only API details (no fofndir), when
   Backup is called, then:
   - Only the ibackup API set is created (existing behaviour unchanged).
   - No fofn subdirectory is created.

4. Given a directory with frequency 0 (frozen), when Backup is called with
   fofndir configured, then the config.yml has freeze=true.

5. Given two servers (one with fofndir "/watch1/", one with fofndir "/watch2/")
   matching different paths, when Backup is called for directories under each
   path, then each directory's fofn is created under the correct fofndir.

6. Given a fofn subdirectory that already exists with requestor=userA and whose
   mtime is within the frequency window, when Backup is called for the same
   directory now claimed by userB, then the FOFN file is NOT rewritten
   (frequency gated) BUT the config.yml IS updated with requestor=userB.

7. Given a fofn subdirectory that was previously created for a claimed
   directory, when the directory is unclaimed and Backup is run again (the
   directory no longer appears in the plan DB), then the fofn subdirectory still
   exists and is not deleted.

### C2: Fofndir-only server handling in New()

As a developer, I want `ibackup.New()` to handle server entries that have only a
FofnDir and no Addr, so that fofndir-only configurations work without connection
errors.

When `createServers()` encounters a server entry where `Addr` is empty, it must
skip the `connect()` call and not start an `await` goroutine. A nil
`serverClient` is stored for the server so that `createClients()` can still
build the path-to-server routing. `MultiClient.Backup()` must skip API
operations (GetSetByName, AddOrUpdateSet, MergeFiles, TriggerDiscovery) when the
underlying client is nil, but still allow the caller to proceed with fofn
writing.

**Package:** `ibackup/`
**File:** `ibackup/ibackup.go`
**Test file:** `ibackup/ibackup_test.go`

**Acceptance tests:**

1. Given a Config with a server entry that has FofnDir set but Addr, Cert,
   Token, and Username all empty, when New() is called, then no error is
   returned and the returned MultiClient is non-nil.

2. Given a Config with a server entry that has both Addr and FofnDir set, when
   New() is called, then the server connection is attempted normally (existing
   behaviour).

3. Given a MultiClient created with a fofndir-only server config, when Backup()
   is called for a path matching that server with a non-empty file list, then no
   API call is attempted, no panic or nil-pointer error occurs, and no error is
   returned.

4. Given a Config with one fofndir-only server and one API-only server matching
   different paths, when New() is called, then only the API server has a
   connection attempt and the fofndir-only server does not generate a connection
   error.

### C3: Config parsing for FofnDir field

As a developer, I want the config parser to correctly read the `fofndir`
property from server entries, so that fofndir-based backup can be configured.

**Package:** `config/`
**File:** `config/config.go`
**Test file:** `config/config_test.go`

**Acceptance tests:**

1. Given a config YAML with a server entry containing `fofndir: /watch/`, when
   the config is parsed, then `GetFofnDirs()` returns a map containing the
   fofndir value for the matching path regexp.

2. Given a config YAML with no fofndir on any server entry, when the config is
   parsed, then `GetFofnDirs()` returns an empty map.

3. Given a config YAML with a server that has both API details (addr, cert,
   token, username) and fofndir, when the config is parsed, then both
   `GetIBackupClient()` returns a working MultiClient and `GetFofnDirs()`
   returns the fofndir value.

---

## Section D: Server Status Integration

### D1: Exposing fofn status in the summary endpoint

As a server operator, I want the summary endpoint to include fofn-based backup
status alongside ibackup API status, so that the frontend can display both.

In `backend/report.go`, the `populateIbackupStatus` flow is extended: for each
claimed directory with BackupIBackup rules, if a fofndir is configured for the
path, the fofn status is retrieved via `MultiCache.GetFofnBackupActivity` and
added to the BackupStatus map with a key of `"fofn:" + dir`.

This key is distinct from the existing key (which is just `dir`) so both
statuses appear in the map when both are configured.

**Package:** `backend/`
**File:** `backend/report.go`
**Test file:** `backend/report_test.go`

**Acceptance tests:**

1. Given a claimed directory "/lustre/scratch/a/" with BackupIBackup rules, an
   ibackup API set, and a fofn status file, when the summary endpoint is called,
   then BackupStatus contains two entries:
   - Key `"/lustre/scratch/a/"` with the ibackup API status.
   - Key `"fofn:/lustre/scratch/a/"` with the fofn status including Uploaded,
     Frozen, etc. counts.

2. Given the same setup but no fofn status file exists (watchfofns has not run
   yet), when summary is called, then BackupStatus contains only the ibackup API
   entry (no fofn entry or an entry with zero LastSuccess).

3. Given only fofndir configured (no API details), when summary is called, then
   BackupStatus contains only the fofn entry keyed as
   `"fofn:/lustre/scratch/a/"`.

### D2: Frontend display of fofn status counts

As a user viewing the report, I want to see fofn backup status counts (uploaded,
frozen, failed, etc.) in the backup status table, so that I can understand the
progress of watchfofns-based backups.

The `BackupStatus` TypeScript type is extended with optional count fields
matching `SetBackupActivity`. The report table renders these counts when
present.

**Package:** `frontend/src/`
**File:** `frontend/src/types.ts`, `frontend/src/report.ts`

**Acceptance tests:**

1. Given a BackupStatus object with Uploaded=5, Frozen=3, Failures=1, when the
   report table renders, then the displayed row shows "5" for uploaded, "3" for
   frozen, and "1" for failures.

2. Given a BackupStatus object with all count fields zero or absent, when the
   report table renders, then no extra count columns appear
   (backwards-compatible with existing ibackup API status).

---

## Section E: Integration Tests

### E1: End-to-end backup with fofn files

As a developer, I want an integration test that verifies the full backup-plans
backup command creates fofn files alongside ibackup sets, so that the feature
works end-to-end.

**Package:** root
**File:** `main_test.go`

**Acceptance tests:**

1. Given a config with both API details and a fofndir, a plan DB with a claimed
   directory, and a tree DB, when the backup command is run, then:
   - The CLI output includes the set info message.
   - The ibackup set exists on the server.
   - The fofn subdirectory exists under fofndir.
   - The fofn file contains the expected file paths (null-terminated).
   - The config.yml has the correct transformer, freeze flag, and metadata.

2. Given a config with only fofndir (no API connection details), when the backup
   command is run, then:
   - No ibackup API error occurs.
   - The fofn subdirectory is created correctly.
   - The CLI output includes the set info message.

3. Given a config with fofndir configured and a pre-existing status file in the
   fofn subdirectory (created to simulate `ibackup watchfofns` output), when the
   server's summary endpoint is called, then BackupStatus contains a
   "fofn:<dir>" entry with the correct status counts (Uploaded, Failures, etc.)
   parsed from the status file.

---

## Implementation Order

### Phase 1: Foundation - ibackup dependency and types

Stories: A1

Upgrade `go.mod` to ibackup v1.16.0. Implement `SafeName` function. These have
no external dependencies and can be fully unit-tested.

### Phase 2: FOFN directory writing

Stories: A2, A3 (sequential - A2 then A3)

Implement `FofnDirWriter.Write` with FOFN file creation and config.yml writing.
Then add frequency gating. A3 depends on A2 for the basic write functionality.

### Phase 3: Config update and status reading

Stories: A4, B1 (parallel)

A4 (UpdateConfig) and B1 (FofnStatusReader) are independent of each other and
can be implemented in parallel.

### Phase 4: Caching and config integration

Stories: B2, C2, C3, C1 (sequential - B2 then C2 and C3 in parallel, then C1)

B2 extends MultiCache with fofn status caching. C2 modifies
New()/createServers() to handle fofndir-only servers. C3 tests config parsing
for the FofnDir field. C1 wires fofn writing into the backup command, which
depends on C2 for fofndir-only support and C3 for config parsing.

### Phase 5: Server and frontend integration

Stories: D1, D2 (sequential - D1 then D2)

D1 adds fofn status to the summary endpoint. D2 extends the frontend to display
the counts. D2 depends on D1 for the data format.

### Phase 6: Integration tests

Stories: E1

End-to-end tests that exercise the full flow. Depends on all prior phases.

---

## Appendix: Key Decisions

### Skills

The go-implementor skill (`.github/skills/go-implementor/SKILL.md`) provides the
TDD cycle, copyright boilerplate, code quality standards, and implementation
workflow. The go-reviewer skill (`.github/skills/go-reviewer/SKILL.md`) provides
the review checklist. Phase files reference these skills.

### Existing code reuse

- **`ibackup.MultiClient`** and **`ibackup.MultiCache`** (`ibackup/ibackup.go`):
  Extended with fofndir support. The path-regex routing is reused to determine
  which server (and thus which fofndir) to use for a given directory.
- **`ibackup.SetBackupActivity`** (`ibackup/ibackup.go`): Extended with count
  fields. Reused as the common status type for both ibackup API and fofn-based
  status.
- **`backups.Backup`** (`backups/backups.go`): Extended to also write fofn
  directories. Reuses the existing set name construction (`setNamePrefix +
  dir.Path`).
- **`backups.setNamePrefix`** (`backups/backups.go`): The constant `"plan::"` is
  reused for fofn directory naming.
- **`fofn.WriteConfig`** (`github.com/wtsi-hgi/ibackup/fofn`): Used directly to
  write config.yml files.
- **`fofn.ParseStatus`** (`github.com/wtsi-hgi/ibackup/fofn`): Used directly to
  read status files.
- **`fofn.ReadConfig`** (`github.com/wtsi-hgi/ibackup/fofn`): Used in tests to
  verify written config files.
- **`db.BackupIBackup`** (`db/rules.go`): Reused as the BackupType constant for
  fofn-based backups (fofn is just a different transport for the same ibackup
  system).
- **`internal/plandb`** (`internal/plandb/plandb.go`): Test helper for creating
  populated plan databases.
- **`internal/ibackup`** (`internal/ibackup/ibackup.go`): Test helper for
  creating ibackup test servers.
- **`config.Config`** (`config/config.go`): Extended with fofndir config parsing
  and getter methods.
- **`backend/report.go`** (`backend/report.go`): `populateIbackupStatus` flow
  extended for fofn status.
- **`time.DateOnly`** (Go stdlib): Used for formatting review and remove dates
  as YYYY-MM-DD.

### Error handling

| Category | Policy |
|---|---|
| Missing fofndir base dir | Return error; caller logs and continues |
| Permission error on fofn write | Return error; caller logs and continues |
| Missing status file | Return nil activity, no error |
| Malformed status file | Return parse error; caller logs, uses nil |
| ibackup API connection failure | Existing behaviour (log warning, retry) |
| Empty file iterator | Return early (false, nil); no directory created |
| Invalid transformer (empty) | fofn.WriteConfig returns error |
| Metadata key with colon | fofn.WriteConfig returns error |

### Testing strategy

| Package | Strategy |
|---|---|
| `ibackup/fofndir.go` | Unit tests using `t.TempDir()` for baseDir. No external dependencies. GoConvey style. Includes memory-bounded test for streaming write (A2 test 5). |
| `ibackup/fofnstatus.go` | Unit tests with hand-crafted status files in `t.TempDir()`. GoConvey style. |
| `config/config.go` | Extend existing config tests with fofndir field. |
| `backups/backups.go` | Unit tests with mock ibackup server and temp fofndir. Verify both API sets and fofn files are created. |
| `backend/report.go` | Test with pre-created status files and verify summary output includes fofn status. |
| `frontend/src/` | Manual or automated browser tests (TypeScript changes are display-only). |
| `main_test.go` | Integration tests using real ibackup test server, real plan DB, and temp fofndir. No mocks. |

### Backward compatibility

- The FofnDir config field is optional. Existing configs without fofndir
  continue to work unchanged.
- The new fields on SetBackupActivity use `json:",omitempty"` so existing API
  consumers see no change when the fields are zero.
- The `BackupIBackup` constant is reused for fofn backups; no new BackupType
  constant is introduced.
- When a directory is unclaimed and its fofn subdirectory exists, the
  subdirectory is left in place (no cleanup).
- The frontend changes are additive - existing backup status display is
  unchanged; new count columns only appear when count data is present.

### Boilerplate and TDD

See the go-implementor skill (`.github/skills/go-implementor/SKILL.md`) for
copyright header requirements and the TDD cycle steps.
