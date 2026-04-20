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
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/backups"
	"github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/memtree"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
)

func TestEndpoints(t *testing.T) {
	Convey("Given an ibackup server with backed up sets", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		var u userHandler

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		treeFile := filepath.Join(t.TempDir(), "tree")

		treeNode, dFn, err := memtree.Tree2MemTree(plandb.ExampleTree(), treeFile)
		So(err, ShouldBeNil)

		Reset(dFn)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, config.NewConfig(t, nil, nil, nil, 0, nil))
		So(err, ShouldBeNil)

		So(s.AddTree(treeFile), ShouldBeNil)

		setInfos, err := backups.Backup(testDB, treeNode, s.config.GetIBackupClient())
		So(err, ShouldBeNil)
		So(setInfos, ShouldNotBeNil)

		Convey("You can use the setExists endpoint to retrieve whether a set with a given name exists", func() {
			u = userA
			code, resp := getResponse(
				s.SetExists,
				"/api/setExists?dir=/lustre/&metadata="+setNamePrefix+"/lustre/scratch123/humgen/a/b/",
				nil,
			)

			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "true\n")

			u = "userB"
			code, resp = getResponse(
				s.SetExists,
				"/api/setExists?dir=/lustre/&metadata="+setNamePrefix+"/lustre/scratch123/humgen/a/b/",
				nil,
			)

			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "false\n")
		})
	})
}

func getResponse(fn http.HandlerFunc, u string, body any) (int, string) {
	w := httptest.NewRecorder()

	var req *http.Request

	switch body := body.(type) {
	case url.Values:
		req = httptest.NewRequest(http.MethodPost, u, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	case io.Reader:
		req = httptest.NewRequest(http.MethodPost, u, body)
	default:
		req = httptest.NewRequest(http.MethodGet, u, nil)
	}

	fn(w, req)

	return w.Code, w.Body.String()
}

func checkErrorResponse(t *testing.T, code int, resp string, err Error) {
	t.Helper()

	So(resp, ShouldEqual, err.Error()+"\n")
	So(code, ShouldEqual, err.Code)
}

func (s *Server) stop() {
	s.exit()
	s.gitCache.Stop()
}
