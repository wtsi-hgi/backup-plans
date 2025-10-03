package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unsafe"

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

type reportRoots []string

func (r reportRoots) String() string {
	return fmt.Sprint([]string(r))
}

func (r *reportRoots) Set(val string) error {
	*r = append(*r, val)

	return nil
}

func run() error {
	var (
		port   uint64
		report reportRoots
	)

	flag.Uint64Var(&port, "port", 12345, "port to start server on")
	flag.Var(&report, "report", "reporting root, can be supplied more than once")

	flag.Parse()

	d, err := db.Init("mysql", os.Getenv("BACKUP_MYSQL_URL"))
	if err != nil {
		return err
	}

	s, err := server.New(d, getUser, report)
	if err != nil {
		return err
	}

	for _, db := range flag.Args() {
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
	http.Handle("/api/report/summary", http.HandlerFunc(s.Summary))
	http.Handle("/", http.HandlerFunc(frontend.Serve))

	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func getUser(r *http.Request) string {
	for _, cookie := range r.Cookies() {
		if cookie.Name == "nginxauth" {
			data, err := base64.StdEncoding.DecodeString(cookie.Value)
			if err != nil {
				return ""
			}

			return strings.SplitN(unsafe.String(unsafe.SliceData(data), len(data)), ":", 2)[0]
		}
	}

	return ""
}
