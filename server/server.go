/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package server

import (
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/wtsi-hgi/backup-plans/backend"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/frontend"
	ib "github.com/wtsi-hgi/ibackup/server"
	wrs "github.com/wtsi-hgi/wrstat-ui/server"
)

var dbCheckTime = time.Minute //nolint:gochecknoglobals

// Start creates and start a new server after loading the trees given.
func Start(listen string, d *db.DB, getUser func(*http.Request) string,
	report []string, adminGroup uint32, client *ib.Client, initialTrees ...string) error {
	l, err := net.Listen("tcp", listen) //nolint:noctx
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	return start(l, d, getUser, report, adminGroup, client, initialTrees...)
}

func start(listen net.Listener, d *db.DB, getUser func(*http.Request) string,
	report []string, adminGroup uint32, client *ib.Client, initialTrees ...string) error {
	b, err := backend.New(d, getUser, report, client)
	if err != nil {
		return err
	}

	b.SetAdminGroup(adminGroup)

	err = loadTrees(initialTrees, b)
	if err != nil {
		return err
	}

	return addHandlesAndListen(b, listen)
}

func addHandlesAndListen(b *backend.Server, listen net.Listener) error {
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

	slog.Info("Serving...")

	return http.Serve(listen, nil) //nolint:gosec
}

func loadTrees(initialTrees []string, b *backend.Server) error {
	if len(initialTrees) != 1 {
		loadDBs(b, initialTrees)
	}

	path := initialTrees[0]

	stat, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !stat.IsDir() {
		loadDBs(b, initialTrees)
	}

	treePaths, err := getTreePaths(path)
	if err != nil {
		return err
	}

	loadDBs(b, treePaths)

	go timerLoop(path, b, treePaths)

	return nil
}

func loadDBs(b *backend.Server, trees []string) {
	for _, db := range trees {
		loadDB(b, db)
	}
}

func loadDB(b *backend.Server, db string) {
	slog.Info("Loading Tree", "db", db)

	if err := b.AddTree(db); err != nil {
		slog.Error("Error loading db", "db", err)
	}
}

// getTreePaths will, for a given dir, return a slice of filepaths to all
// 'tree.db' files.
func getTreePaths(path string) ([]string, error) {
	paths, err := wrs.FindDBDirs(path, "tree.db")
	if err != nil {
		return nil, err
	}

	trees := make([]string, 0, len(paths))

	for _, db := range paths {
		trees = append(trees, db+"/tree.db")
	}

	return trees, nil
}

// timerLoop will, given a path to a directory, check for and load all new trees
// in the directory.
func timerLoop(path string, b *backend.Server, treePaths []string) {
	for {
		time.Sleep(dbCheckTime)

		newPaths, err := getTreePaths(path)
		if err != nil {
			slog.Error("Error getting tree paths", "Error", err)

			continue
		}

		for _, path := range newPaths {
			if slices.Contains(treePaths, path) {
				continue
			}

			loadDB(b, path)

			treePaths = append(treePaths, path)
		}
	}
}
