package main

import (
	"fmt"
	"net/http"
	"os"

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
	s := server.New()

	for _, db := range os.Args[1:] {
		fmt.Println("Loading ", db)

		if err := s.AddTree(db); err != nil {
			return err
		}
	}

	fmt.Println("Serving...")

	http.Handle("/api/tree", http.HandlerFunc(s.Tree))
	http.Handle("/", http.HandlerFunc(frontend.Serve))

	return http.ListenAndServe(":12345", nil)
}
