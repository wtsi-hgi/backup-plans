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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/backups"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ib "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
)

func TestSetExists(t *testing.T) {
	Convey("Given an ibackup server with backed up sets", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		s, addr, certPath, dfn, err := ib.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		var u userHandler

		ibackupClient, err := ibackup.Connect(addr, certPath)
		So(err, ShouldBeNil)

		So(s, ShouldNotBeNil)
		So(u, ShouldNotBeNil)
		So(ibackupClient, ShouldNotBeNil)

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := plandb.ExampleTree()

		server, err := New(testdb.CreateTestDatabase(t), u.getUser, nil, ibackupClient)
		So(err, ShouldBeNil)

		setInfos, err := backups.Backup(testDB, tr, ibackupClient)
		So(err, ShouldBeNil)
		So(setInfos, ShouldNotBeNil)

		Convey("You can use the setExists endpoint to retrieve whether a set with a given name exists", func() {
			u = "userA"
			code, resp := getResponse(
				server.SetExists,
				"/api/setExists?metadata=plan::/lustre/scratch123/humgen/a/b/",
				nil,
			)

			So(code, ShouldEqual, http.StatusOK)
			So(resp, ShouldEqual, "true\n")

			u = "userB"
			code, resp = getResponse(
				server.SetExists,
				"/api/setExists?metadata=plan::/lustre/scratch123/humgen/a/b/",
				nil,
			)

			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldEqual, "")
		})
	})
}

func getResponse(fn http.HandlerFunc, url string, body io.Reader) (int, string) {
	w := httptest.NewRecorder()

	fn(w, httptest.NewRequest(http.MethodGet, url, body))

	return w.Code, w.Body.String()
}
