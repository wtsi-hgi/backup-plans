package server

import (
	"log/slog"
	"net/http"

	"github.com/wtsi-hgi/backup-plans/backend"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/frontend"
)

// Start creates and start a new server after loading the trees given.
func Start(listen string, d *db.DB, getUser func(*http.Request) string, report []string, initialTrees ...string) error {
	b, err := backend.New(d, getUser, report)
	if err != nil {
		return err
	}

	for _, db := range initialTrees {
		slog.Info("Loading", "db", db)

		if err := b.AddTree(db); err != nil {
			return err
		}
	}

	slog.Info("Serving")

	http.Handle("/api/whoami", http.HandlerFunc(b.WhoAmI))
	http.Handle("/api/tree", http.HandlerFunc(b.Tree))
	http.Handle("/api/dir/claim", http.HandlerFunc(b.ClaimDir))
	http.Handle("/api/dir/pass", http.HandlerFunc(b.PassDirClaim))
	http.Handle("/api/dir/revoke", http.HandlerFunc(b.RevokeDirClaim))
	http.Handle("/api/rules/create", http.HandlerFunc(b.CreateRule))
	http.Handle("/api/rules/update", http.HandlerFunc(b.UpdateRule))
	http.Handle("/api/rules/remove", http.HandlerFunc(b.RemoveRule))
	http.Handle("/api/report/summary", http.HandlerFunc(b.Summary))
	http.Handle("/", frontend.Index)

	return http.ListenAndServe(listen, nil)
}
