/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *         Sky Haines <sh55@sanger.ac.uk>
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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/users"
	"vimagination.zapto.org/tree"
)

func TestSeverDBUpdate(t *testing.T) {
	Convey("Given a directory containing tree dbs", t, func() {
		dirA := directories.NewRoot("/some/path/", 1000)
		dirB := directories.NewRoot("/some/other/path/", 2000)

		u, err := user.Current()
		So(err, ShouldBeNil)

		uid, gids := users.GetIDs(u.Username)

		file := directories.AddFile(&dirA.Directory, "myData/myFile.txt", uid, gids[0], 5, 1000)
		directories.AddFile(&dirB.Directory, ".bin/exe", 0, 0, 1234, 1000)

		tmp := t.TempDir()

		writeDB(t, dirA, tmp, "001_somePath")
		writeDB(t, dirB, tmp, "001_otherPath")

		Convey("You can create a server", func() {
			l, err := net.Listen("tcp", ":0") //nolint:gosec,noctx
			So(err, ShouldBeNil)

			tdb := testdb.CreateTestDatabase(t)
			errCh := make(chan error)
			dbCheckTime = time.Second

			client, err := ibackup.New(ibackup.Config{})
			So(err, ShouldBeNil)

			go func() {
				errCh <- start(l, tdb, func(*http.Request) string { return u.Username }, nil, 0, client, "", "", tmp)
			}()

			baseURL := fmt.Sprintf("http://127.0.0.1:%d/", l.Addr().(*net.TCPAddr).Port) //nolint:errcheck,forcetypeassert
			tick := time.NewTicker(time.Second)

		Loop:
			for {
				select {
				case err = <-errCh:
					So(err, ShouldBeNil)
				case <-tick.C:
					_, err = http.Get(baseURL) //nolint:noctx,gosec
					if err == nil {
						break Loop
					}
				}
			}

			tick.Stop()

			Convey("You can update a database", func() {
				r, err := http.Get(baseURL + "api/tree?dir=/some/path/myData/") //nolint:noctx
				So(err, ShouldBeNil)

				var buf strings.Builder

				_, err = io.Copy(&buf, r.Body)
				So(err, ShouldBeNil)
				So(buf.String(), ShouldContainSubstring, `"Files":1`)
				So(buf.String(), ShouldContainSubstring, `"Size":5`)
				So(buf.String(), ShouldNotContainSubstring, `"Files":2`)

				buf.Reset()

				file.Size = 1

				directories.AddFile(&dirA.Directory, "myData/myFile2.txt", uid, gids[0], 10, 1000)
				writeDB(t, dirA, tmp, "002_somePath")

				time.Sleep(time.Second * 2)

				r, err = http.Get(baseURL + "api/tree?dir=/some/path/myData/") //nolint:noctx
				So(err, ShouldBeNil)

				_, err = io.Copy(&buf, r.Body)
				So(err, ShouldBeNil)
				So(buf.String(), ShouldContainSubstring, `"Files":2`)
				So(buf.String(), ShouldContainSubstring, `"Size":11`)
				So(buf.String(), ShouldNotContainSubstring, `"Files":1`)
			})
		})
	})
}

func writeDB(t *testing.T, root *directories.Root, base, path string) {
	t.Helper()

	So(os.Mkdir(filepath.Join(base, path), 0700), ShouldBeNil)

	db := filepath.Join(base, path, "tree.db")

	f, err := os.Create(db)
	So(err, ShouldBeNil)
	So(tree.Serialise(f, root), ShouldBeNil)
	So(f.Close(), ShouldBeNil)
}
