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

package wrstat

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/wrstat-ui/basedirs"
	"github.com/wtsi-hgi/wrstat-ui/db"
	"github.com/wtsi-hgi/wrstat-ui/server"
	"github.com/wtsi-hgi/wrstat-ui/stats"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	sbasedirs "github.com/wtsi-hgi/wrstat-ui/summary/basedirs"
	"github.com/wtsi-hgi/wrstat-ui/summary/dirguta"
)

const (
	jwtBasename         = "jwt"
	serverTokenBasename = "st"
	basedirsPath        = "basedirs.db"
	dgutaPath           = "dguta.dbs"
)

func TestWRStat(t *testing.T) {
	Convey("With a wrstat database and server", t, func() {
		s := summary.NewSummariser(stats.NewStatsParser(plandb.ExampleTree().AsReader()))

		tmp := t.TempDir()
		run := filepath.Join(tmp, "20251225-000000_root")
		dguta := filepath.Join(run, dgutaPath)
		owners := filepath.Join(tmp, "owners")

		t.Setenv("XDG_STATE_HOME", tmp)

		So(os.MkdirAll(dguta, 0700), ShouldBeNil)
		So(os.WriteFile(owners, nil, 0600), ShouldBeNil)

		db := db.NewDB(dguta)

		So(db.CreateDB(), ShouldBeNil)

		db.SetBatchSize(100)
		s.AddDirectoryOperation(dirguta.NewDirGroupUserTypeAge(db))

		bd, err := basedirs.NewCreator(filepath.Join(run, basedirsPath), nil)
		So(err, ShouldBeNil)

		bd.SetMountPoints([]string{"/"})
		bd.SetModTime(time.Now())

		s.AddDirectoryOperation(sbasedirs.NewBaseDirs(func(*summary.DirectoryPath) bool { return false }, bd))

		So(s.Summarise(), ShouldBeNil)
		So(db.Close(), ShouldBeNil)

		cert, key, err := gas.CreateTestCert(t)
		So(err, ShouldBeNil)

		srv := server.New(io.Discard)
		srv.WhiteListGroups(func(_ string) bool { return true })
		So(srv.EnableAuth(cert, key, func(_, _ string) (bool, string) { return true, "0" }), ShouldBeNil)
		So(srv.LoadDBs([]string{run}, dgutaPath, basedirsPath, owners, "/"), ShouldBeNil)

		addr, dfunc, err := gas.StartTestServer(srv, cert, key)
		So(err, ShouldBeNil)

		c, err := gas.NewClientCLI(jwtBasename, serverTokenBasename, addr, cert, false)
		So(err, ShouldBeNil)
		So(c.Login("user", "password"), ShouldBeNil)

		Reset(func() {
			So(dfunc(), ShouldBeNil)
		})

		Convey("With a configured WRStat client", func() {
			cfg := Config{
				JWTBasename:         jwtBasename,
				ServerTokenBasename: serverTokenBasename,
				ServerURL:           addr,
				ServerCert:          cert,
			}

			client, err := New(time.Hour, cfg)
			So(err, ShouldBeNil)

			Reset(client.Stop)

			Convey("You can request mtimes", func() {
				ts, err := client.GetWRStatModTime("/lustre/scratch123/humgen/a/b/")
				So(err, ShouldBeNil)
				So(ts.Unix(), ShouldEqual, 98767)

				ts, err = client.GetWRStatModTime("/lustre/scratch123/humgen//a/b")
				So(err, ShouldBeNil)
				So(ts.Unix(), ShouldEqual, 98767)

				ts, err = client.GetWRStatModTime("/not/a/path/")
				So(err, ShouldNotBeNil)
				So(ts, ShouldBeZeroValue)
			})
		})
	})
}
