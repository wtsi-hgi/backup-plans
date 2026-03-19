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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/backups"
	"github.com/wtsi-hgi/backup-plans/config"
	iconfig "github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"github.com/wtsi-hgi/ibackup/fofn"
	"github.com/wtsi-hgi/ibackup/set"
	"vimagination.zapto.org/tree"
)

func TestEndpoints(t *testing.T) {
	Convey("Given an ibackup server with backed up sets", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		var u userHandler

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := plandb.ExampleTree()

		treeFile := filepath.Join(t.TempDir(), "tree.db")
		f, err := os.Create(treeFile)
		So(err, ShouldBeNil)

		So(tree.Serialise(f, tr), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		s, err := New(testdb.CreateTestDatabase(t), u.getUser, iconfig.NewConfig(t, nil, nil, nil, 0))
		So(err, ShouldBeNil)

		So(s.AddTree(treeFile), ShouldBeNil)

		setInfos, err := backups.Backup(testDB, tr, s.config.GetIBackupClient())
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

func TestThawRefreeze(t *testing.T) {
	Convey("Given a configured server with an ibackup server", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		var u userHandler

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := plandb.ExampleTree()

		treeFile := filepath.Join(t.TempDir(), "tree.db")
		f, err := os.Create(treeFile)
		So(err, ShouldBeNil)

		So(tree.Serialise(f, tr), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		cfg := filepath.Join(t.TempDir(), "config.yaml")
		fofnDir := t.TempDir()

		So(os.WriteFile(cfg, []byte(""+
			`ibackup:
  servers:
    "server":
      fofndir: `+fofnDir+`
  pathtoserver:
    ^/:
      servername: server
      transformer: prefix=/:/
ibackupcacheduration: 3600`,
		), 0600), ShouldBeNil)

		c, err := config.Parse(cfg)
		So(err, ShouldBeNil)

		s, err := New(testDB, u.getUser, c)
		So(err, ShouldBeNil)

		Reset(s.exit)

		So(s.AddTree(treeFile), ShouldBeNil)

		Convey("You can thaw a frozen set", func() {
			code, resp := getResponse(s.Thaw, "/api/thaw", url.Values{"dir": {"/not/a/dir"}})
			checkErrorResponse(t, code, resp, ErrInvalidDir)

			code, resp = getResponse(s.Thaw, "/api/thaw", url.Values{"dir": {"/scratch123/humgen/a/b/"}})
			checkErrorResponse(t, code, resp, ErrDirectoryNotClaimed)

			code, resp = getResponse(s.Thaw, "/api/thaw", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}})
			checkErrorResponse(t, code, resp, ErrInvalidUser)

			u = "userA"

			code, resp = getResponse(s.Thaw, "/api/thaw", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}})
			checkErrorResponse(t, code, resp, ErrDirectoryNotFrozen)

			now := time.Now().Unix()

			code, resp = getResponse(s.SetDirDetails, "/api/setDetails", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}, "frequency": {"1"}, "review": {strconv.FormatInt(now+1000, 10)}, "remove": {strconv.FormatInt(now+2000, 10)}, "frozen": {"true"}})
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldBeBlank)

			So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze, ShouldEqual, time.Unix(0, 0))

			code, resp = getResponse(s.Thaw, "/api/thaw", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}})
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldBeBlank)

			So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze.Unix(), ShouldBeGreaterThanOrEqualTo, now)

			ns, err := New(testDB, u.getUser, c)
			So(err, ShouldBeNil)
			So(ns.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze.Unix(), ShouldBeGreaterThanOrEqualTo, now)

			u = "root"

			code, resp = getResponse(s.Refreeze, "/api/refreeze", url.Values{"dir": {"/not/a/dir"}})
			checkErrorResponse(t, code, resp, ErrInvalidDir)

			code, resp = getResponse(s.Refreeze, "/api/refreeze", url.Values{"dir": {"/scratch123/humgen/a/b/"}})
			checkErrorResponse(t, code, resp, ErrDirectoryNotClaimed)

			code, resp = getResponse(s.Refreeze, "/api/refreeze", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}})
			checkErrorResponse(t, code, resp, ErrInvalidUser)

			u = "userA"

			code, resp = getResponse(s.Refreeze, "/api/refreeze", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}})
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldBeBlank)

			So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze, ShouldEqual, time.Unix(0, 0))

			ns, err = New(testDB, u.getUser, c)
			So(err, ShouldBeNil)
			So(ns.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze, ShouldEqual, time.Unix(0, 0))

			code, resp = getResponse(s.Refreeze, "/api/refreeze", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}})
			checkErrorResponse(t, code, resp, ErrAlreadyFrozen)
		})
	})
}

func TestThawRefreezeBackup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		Convey("Given a configured server with an ibackup server", t, func() {
			So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

			u := userHandler("userA")

			testDB, _ := plandb.PopulateExamplePlanDB(t)
			tr := plandb.ExampleTree()

			treeFile := filepath.Join(t.TempDir(), "tree.db")
			f, err := os.Create(treeFile)
			So(err, ShouldBeNil)

			So(tree.Serialise(f, tr), ShouldBeNil)
			So(f.Close(), ShouldBeNil)

			cfg := filepath.Join(t.TempDir(), "config.yaml")
			fofnDir := t.TempDir()

			So(os.WriteFile(cfg, []byte(""+
				`ibackup:
  servers:
    "server":
      fofndir: `+fofnDir+`
  pathtoserver:
    ^/:
      servername: server
      transformer: prefix=/:/
ibackupcacheduration: 3600`,
			), 0600), ShouldBeNil)

			c, err := config.Parse(cfg)
			So(err, ShouldBeNil)

			s, err := New(testDB, u.getUser, c)
			So(err, ShouldBeNil)

			Reset(s.stop)
			Reset(s.config.GetCachedIBackupClient().Stop)
			Reset(s.config.GetIBackupClient().Stop)

			So(s.AddTree(treeFile), ShouldBeNil)

			now := time.Now().Unix()

			code, resp := getResponse(s.SetDirDetails, "/api/setDetails", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}, "frequency": {"1"}, "review": {strconv.FormatInt(now+1000, 10)}, "remove": {strconv.FormatInt(now+2000, 10)}, "frozen": {"true"}})
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldBeBlank)

			So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze, ShouldEqual, time.Unix(0, 0))

			code, resp = getResponse(s.Thaw, "/api/thaw", url.Values{"dir": {"/lustre/scratch123/humgen/a/b/"}})
			So(code, ShouldEqual, http.StatusNoContent)
			So(resp, ShouldBeBlank)

			So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze.Unix(), ShouldBeGreaterThanOrEqualTo, now)

			ns, err := New(testDB, u.getUser, c)
			So(err, ShouldBeNil)
			So(ns.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze.Unix(), ShouldBeGreaterThanOrEqualTo, now)

			ns.stop()

			fofnPath := filepath.Join(fofnDir, (&set.Set{Requester: "userA", Name: setNamePrefix + "/lustre/scratch123/humgen/a/b/"}).ID())

			So(os.MkdirAll(fofnPath, 0700), ShouldBeNil)
			So(fofn.WriteConfig(fofnPath, fofn.SubDirConfig{
				Transformer: "prefix=/:/",
				Freeze:      true,
				Requester:   "userA",
				Name:        setNamePrefix + "/lustre/scratch123/humgen/a/b/",
			}), ShouldBeNil)
			So(os.WriteFile(filepath.Join(fofnPath, "status"), nil, 0600), ShouldBeNil)

			time.Sleep(61 * time.Minute)

			So(s.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze, ShouldEqual, time.Unix(0, 0))

			ns, err = New(testDB, u.getUser, c)
			So(err, ShouldBeNil)
			So(ns.directoryRules["/lustre/scratch123/humgen/a/b/"].Unfreeze, ShouldEqual, time.Unix(0, 0))

			ns.stop()
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
