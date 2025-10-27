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
	"context"
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
	"github.com/wtsi-hgi/wrstat-ui/server"
)

var dbCheckTime = time.Minute //nolint:gochecknoglobals

// Start creates and start a new server after loading the trees given.
func Start(listen string, d *db.DB, getUser func(*http.Request) string,
	report []string, adminGroup uint32, initialTrees ...string) error {
	// l, err := net.Listen("tcp", listen)
	var lc net.ListenConfig

	l, err := lc.Listen(context.Background(), "tcp", listen)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	return start(l, d, getUser, report, adminGroup, initialTrees...)
}

func start(listen net.Listener, d *db.DB, getUser func(*http.Request) string,
	report []string, adminGroup uint32, initialTrees ...string) error {
	b, err := backend.New(d, getUser, report)
	if err != nil {
		return err
	}

	b.SetAdminGroup(adminGroup)

	trees, err := loadTrees(initialTrees, b)
	if err != nil {
		return err
	}

	// If initialTrees is a list of files, load them
	for _, db := range trees {
		err = loadDBs(db, b)
		if err != nil {
			slog.Error("Error loading db", "db", err)

			continue
		}
	}

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

	return http.Serve(listen, nil) //nolint:gosec
}

func loadTrees(initialTrees []string, b *backend.Server) ([]string, error) {
	if len(initialTrees) != 1 {
		return initialTrees, nil
	}

	path := initialTrees[0]

	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		return initialTrees, nil
	}

	treePaths, err := getTreePaths(path)
	if err != nil {
		return nil, err
	}

	go timerLoop(path, b, treePaths)

	return treePaths, nil
}

// getTreePaths will, for a given dir, return a slice of filepaths to all
// 'tree.db' files.
func getTreePaths(path string) ([]string, error) {
	paths, err := server.FindDBDirs(path, "tree.db")
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
// TODO: Is it possible to remove a tree? If so, how should this be handled?
func timerLoop(path string, b *backend.Server, treePaths []string) { //nolint:gocognit
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

			err = loadDBs(path, b)
			if err != nil {
				slog.Error("Error loading db", "db", path, "Error", err)

				continue
			}

			treePaths = append(treePaths, path)
		}
	}
}

func loadDBs(path string, b *backend.Server) error {
	if err := b.AddTree(path); err != nil {
		return err
	}

	return nil
}
