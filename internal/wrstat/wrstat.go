package wrstat

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wtsi-hgi/backup-plans/internal/directories"
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

func NewTestWRStatClient(t *testing.T, tree *directories.Root) (*wr.Client, func()) { //nolint:gocognit,gocyclop,gocyclo,cyclop,funlen,lll
	t.Helper()

	s := summary.NewSummariser(stats.NewStatsParser(tree.AsReader()))

	tmp := t.TempDir()
	run := filepath.Join(tmp, "20251225-000000_root")
	dguta := filepath.Join(run, dgutaPath)
	owners := filepath.Join(tmp, "owners")

	t.Setenv("XDG_STATE_HOME", tmp)

	if err := os.MkdirAll(dguta, 0700); err != nil { //nolint:mnd
		t.Fatal(err)
	}

	if err := os.WriteFile(owners, nil, 0600); err != nil { //nolint:mnd
		t.Fatal(err)
	}

	db := db.NewDB(dguta)
	if err := db.CreateDB(); err != nil {
		t.Fatal(err)
	}

	db.SetBatchSize(100) //nolint:mnd
	s.AddDirectoryOperation(dirguta.NewDirGroupUserTypeAge(db))

	bd, err := basedirs.NewCreator(filepath.Join(run, basedirsPath), nil)
	if err != nil {
		t.Fatal(err)
	}

	bd.SetMountPoints([]string{"/"})
	bd.SetModTime(time.Now())

	s.AddDirectoryOperation(
		sbasedirs.NewBaseDirs(func(*summary.DirectoryPath) bool { return false }, bd),
	)

	if err := s.Summarise(); err != nil { //nolint:govet
		t.Fatal(err)
	}

	if err := db.Close(); err != nil { //nolint:govet
		t.Fatal(err)
	}

	cert, key, err := gas.CreateTestCert(t)
	if err != nil {
		t.Fatal(err)
	}

	srv := server.New(io.Discard)
	srv.WhiteListGroups(func(_ string) bool { return true })

	if err := srv.EnableAuth(cert, key, func(_, _ string) (bool, string) { //nolint:govet
		return true, "0"
	}); err != nil {
		t.Fatal(err)
	}

	if err := srv.LoadDBs([]string{run}, dgutaPath, basedirsPath, owners, "/"); err != nil { //nolint:govet
		t.Fatal(err)
	}

	addr, dfunc, err := gas.StartTestServer(srv, cert, key)
	if err != nil {
		t.Fatal(err)
	}

	c, err := gas.NewClientCLI(jwtBasename, serverTokenBasename, addr, cert, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Login("user", "password"); err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		dfunc() //nolint:errcheck
	}

	cfg := wr.Config{
		JWTBasename:         jwtBasename,
		ServerTokenBasename: serverTokenBasename,
		ServerURL:           addr,
		ServerCert:          cert,
	}

	client, _ := wr.New(time.Hour, cfg) //nolint:errcheck

	return client, cleanup
}
