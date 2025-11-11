/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Sky Haines <sh55@sanger.ac.uk>
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

package backend

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/backups"
	ib "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"vimagination.zapto.org/tree"
)

func TestEndpoints(t *testing.T) {
	Convey("Given an ibackup server with backed up sets", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		ibackupClient := ib.NewMultiClient(t)

		var u userHandler

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := plandb.ExampleTree()

		treeFile := filepath.Join(t.TempDir(), "tree.db")
		f, err := os.Create(treeFile)
		So(err, ShouldBeNil)

		So(tree.Serialise(f, tr), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		server, err := New(testdb.CreateTestDatabase(t), u.getUser, nil, ibackupClient, "")
		So(err, ShouldBeNil)

		err = server.AddTree(treeFile)
		So(err, ShouldBeNil)

		setInfos, err := backups.Backup(testDB, tr, ibackupClient)
		So(err, ShouldBeNil)
		So(setInfos, ShouldNotBeNil)

		Convey("You can use the setExists endpoint to retrieve whether a set with a given name exists", func() {
			u = "userA"
			code, resp := getResponse(
				server.SetExists,
				"/api/setExists?dir=/lustre/&metadata=plan::/lustre/scratch123/humgen/a/b/",
				nil,
			)

			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "true\n")

			u = "userB"
			code, resp = getResponse(
				server.SetExists,
				"/api/setExists?dir=/lustre/&metadata=plan::/lustre/scratch123/humgen/a/b/",
				nil,
			)

			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldEqual, "")
		})

		Convey("You can check if a slice of paths refers to directories or files", func() {
			u = "userA"
			code, resp := getResponse(
				server.GetDirectories,
				"/test?",
				strings.NewReader(`[
					"lustre/scratch123/humgen/a/b/",
					"/lustre/scratch123/humgen/a",
					"/lustre/scratch123/humgen/a/b/1.jpg"
				]`),
			)

			So(resp, ShouldEqual, "[true,true,false]\n")
			So(code, ShouldEqual, http.StatusOK)
		})
	})
}

func getResponse(fn http.HandlerFunc, url string, body io.Reader) (int, string) {
	w := httptest.NewRecorder()

	fn(w, httptest.NewRequest(http.MethodGet, url, body))

	return w.Code, w.Body.String()
}
