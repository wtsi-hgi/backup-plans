# Backup Plans

Backup Plans allows users to supply backup plans for their data.

Uses [WRStat](https://github.com/wtsi-ssg/wrstat/) for directory data and
[IBackup](https://github.com/wtsi-hgi/ibackup/) for performing the actual
backups.

# Development

For developing the frontend, the following can be run from the `frontend`
directory:

```bash
BACKUP_PLANS_CONNECTION="sqlite:/path/to/db.sqlite" XDG_STATE_HOME="/path/to/ibackup/token/dir/" go run -tags dev ../main.go server --listen LISTEN_PORT --admin ADMIN_GID --owners /path/to/wrstat/owners/file --bom /path/to/wrstat/bom.areas/file --config /path/to/config.yaml /path/to/tree/dbs/
```

The included script `frontend/embed.sh` can be run to compile the frontend so that a simple `go build` will include the completed frontend.

The generated files need not be included in PRs to the develop branch, as their
is a GitHub action that automatically builds and commits the frontend on a push.