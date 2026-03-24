package wrstat

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey" //nolint:revive,staticcheck
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/wrstat"
	wr "github.com/wtsi-hgi/backup-plans/wrstat"
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

func NewTestWRStatClient(t *testing.T, tree *directories.Root) (*wr.Client, wrstat.Config) { //nolint:funlen
	t.Helper()

	s := summary.NewSummariser(stats.NewStatsParser(tree.AsReader()))

	tmp := t.TempDir()
	run := filepath.Join(tmp, "20251225-000000_root")
	dguta := filepath.Join(run, dgutaPath)
	owners := filepath.Join(tmp, "owners")

	t.Setenv("XDG_STATE_HOME", tmp)

	So(os.MkdirAll(dguta, 0700), ShouldBeNil)        //nolint:mnd
	So(os.WriteFile(owners, nil, 0600), ShouldBeNil) //nolint:mnd

	db := db.NewDB(dguta)

	So(db.CreateDB(), ShouldBeNil)

	db.SetBatchSize(100) //nolint:mnd
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

	cfg := wr.Config{
		JWTBasename:         jwtBasename,
		ServerTokenBasename: serverTokenBasename,
		ServerURL:           addr,
		ServerCert:          cert,
	}

	client, err := wr.New(time.Hour, cfg)
	So(err, ShouldBeNil)

	Reset(client.Stop)

	return client, cfg
}
