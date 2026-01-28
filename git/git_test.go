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
		}

		var mux http.ServeMux

		for repo, commits := range repos {
			r, err := git.Init(memory.NewStorage(), git.WithWorkTree(memfs.New()))
			So(err, ShouldBeNil)

			for n, commit := range commits {
				fname := strconv.Itoa(n)

				wt, err := r.Worktree()
				So(err, ShouldBeNil)

				file, err := wt.Filesystem.Create(fname)
				So(err, ShouldBeNil)

				_, err = file.Write([]byte{byte(n)})
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

			mux.Handle("/"+repo+"/", githttp.NewBackend(&loader{r.Storer}))
		}

		server := httptest.NewServer(&mux)

		Convey("You can query latest commit times", func() {
			for repo, commits := range repos {
				commitTime, err := GetLatestCommitDate(server.URL + "/" + repo + "/")
				So(err, ShouldBeNil)
				So(commitTime, ShouldEqual, commits[len(commits)-1])
			}
		})
	})
}

type loader struct {
	storage.Storer
}

func (l *loader) Load(_ *transport.Endpoint) (storage.Storer, error) {
	return l.Storer, nil
}
