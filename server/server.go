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
	"log/slog"
	"net/http"

	"github.com/wtsi-hgi/backup-plans/backend"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/frontend"
)

// Start creates and start a new server after loading the trees given.
func Start(listen string, d *db.DB, getUser func(*http.Request) string, report []string, adminGroup uint32, initialTrees ...string) error {
	b, err := backend.New(d, getUser, report)
	if err != nil {
		return err
	}

	b.SetAdminGroup(adminGroup)

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
