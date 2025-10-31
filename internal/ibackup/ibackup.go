package ibackup

import (
	"io"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey" //nolint:staticcheck,revive
	"github.com/wtsi-hgi/backup-plans/ibackup"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/baton"
	"github.com/wtsi-hgi/ibackup/server"
)

// NewTestIbackupServer returns a test ibackup server, its address, certificate
// path, a function you should defer to stop the server, and an error.
func NewTestIbackupServer(t *testing.T) (*server.Server, string, string, func() error, error) { //nolint:funlen
	t.Helper()

	handler, err := baton.GetBatonHandler()
	if err != nil {
		return nil, "", "", nil, err
	}

	conf := server.Config{
		HTTPLogger:     io.Discard,
		StorageHandler: handler,
	}

	s, err := server.New(conf)
	if err != nil {
		return nil, "", "", nil, err
	}

	certPath, keyPath, err := gas.CreateTestCert(t)
	if err != nil {
		return nil, "", "", nil, err
	}

	err = s.EnableAuthWithServerToken(certPath, keyPath, ".ibackup.token",
		func(_, _ string) (bool, string) { return true, "1" })
	if err != nil {
		return nil, "", "", nil, err
	}

	if err = s.MakeQueueEndPoints(); err != nil {
		return nil, "", "", nil, err
	}

	if err = s.LoadSetDB(filepath.Join(t.TempDir(), "db"), ""); err != nil {
		return nil, "", "", nil, err
	}

	addr, dfn, err := gas.StartTestServer(s, certPath, keyPath)

	return s, addr, certPath, dfn, err
}

func NewClient(t *testing.T) *server.Client {
	t.Helper()

	_, addr, certPath, dfn, err := NewTestIbackupServer(t)
	So(err, ShouldBeNil)

	Reset(func() { So(dfn(), ShouldBeNil) })

	client, err := ibackup.Connect(addr, certPath)
	So(err, ShouldBeNil)

	return client
}
