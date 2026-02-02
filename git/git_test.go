/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
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

package git

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	githttp "github.com/go-git/go-git/v6/backend/http"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetLatestCommitDate(t *testing.T) {
	Convey("With some test repos", t, func() {
		repos := map[string][]time.Time{
			"A": {
				time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC),
			},
			"B": {
				time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC),
				time.Date(2003, 4, 5, 6, 7, 8, 0, time.UTC),
			},
			"C": {
				time.Date(1990, 1, 1, 1, 1, 1, 0, time.UTC),
			},
		}
		old := time.Date(1999, 1, 1, 1, 1, 1, 0, time.UTC)

		var mux http.ServeMux

		for repo, commits := range repos {
			r, err := git.Init(memory.NewStorage(), git.WithWorkTree(memfs.New()))
			So(err, ShouldBeNil)

			for n, commit := range commits {
				wt, errr := r.Worktree()
				So(errr, ShouldBeNil)

				addCommit(t, wt, strconv.Itoa(n), commit)
			}

			wt, err := r.Worktree()
			So(err, ShouldBeNil)

			So(wt.Checkout(&git.CheckoutOptions{
				Hash:   plumbing.ZeroHash,
				Branch: plumbing.NewBranchReferenceName("old"),
				Create: true,
			}), ShouldBeNil)

			addCommit(t, wt, "old", old)

			So(wt.Checkout(&git.CheckoutOptions{Branch: "refs/heads/master"}), ShouldBeNil)

			mux.Handle("/"+repo+"/", githttp.NewBackend(&loader{r.Storer}))
		}

		server := httptest.NewServer(&mux)

		Convey("You can query latest commit times", func() {
			for repo, commits := range repos {
				latest := commits[len(commits)-1]
				if repo == "C" {
					latest = old
				}

				commitTime, err := GetLatestCommitDate(server.URL + "/" + repo + "/")
				So(err, ShouldBeNil)
				So(commitTime, ShouldEqual, latest)
			}
		})
	})
}

func addCommit(t *testing.T, wt *git.Worktree, fname string, commit time.Time) {
	t.Helper()

	file, err := wt.Filesystem.Create(fname)
	So(err, ShouldBeNil)

	_, err = file.Write([]byte(fname))
	So(err, ShouldBeNil)
	So(file.Close(), ShouldBeNil)

	_, err = wt.Add(fname)
	So(err, ShouldBeNil)

	_, err = wt.Commit("commit "+fname, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "git",
			Email: "git@commit",
			When:  commit,
		},
	})
	So(err, ShouldBeNil)
}

type loader struct {
	storage.Storer
}

func (l *loader) Load(_ *transport.Endpoint) (storage.Storer, error) {
	return l.Storer, nil
}
