package main

import (
	"fmt"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/frontend"
	"github.com/wtsi-hgi/backup-plans/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

func run() error {
	d, err := db.Init("mysql", os.Getenv("BACKUP_MYSQL_URL")+"?parseTime=true")
	if err != nil {
		return err
	}

	s, err := server.New(d, getUser)
	if err != nil {
		return err
	}

	for _, db := range os.Args[1:] {
		fmt.Println("Loading ", db)

		if err := s.AddTree(db); err != nil {
			return err
		}
	}

	fmt.Println("Serving...")

	http.Handle("/api/whoami", http.HandlerFunc(s.WhoAmI))
	http.Handle("/api/tree", http.HandlerFunc(s.Tree))
	http.Handle("/api/dir/claim", http.HandlerFunc(s.ClaimDir))
	http.Handle("/api/dir/pass", http.HandlerFunc(s.PassDirClaim))
	http.Handle("/api/dir/revoke", http.HandlerFunc(s.RevokeDirClaim))
	http.Handle("/api/rules/create", http.HandlerFunc(s.CreateRule))
	http.Handle("/api/rules/get", http.HandlerFunc(s.GetRules))
	http.Handle("/api/rules/update", http.HandlerFunc(s.UpdateRule))
	http.Handle("/api/rules/remove", http.HandlerFunc(s.RemoveRule))
	http.Handle("/", http.HandlerFunc(frontend.Serve))

	return http.ListenAndServe(":12345", nil)
}

func getUser(r *http.Request) string {
	return "mw31"
}
