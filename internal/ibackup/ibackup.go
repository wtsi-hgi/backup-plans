package ibackup

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey" //nolint:staticcheck,revive
	"github.com/wtsi-hgi/backup-plans/ibackup"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/baton"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/ibackup/transformer"
)

const CustomTransformer = "customTransformer"

func init() { //nolint:gochecknoinits
	transformer.Register(CustomTransformer, "^/some/path/", "/remote/path/") //nolint:errcheck
}

// NewTestIbackupServer returns a test ibackup server, its address, certificate
// path, a function you should defer to stop the server, and an error.
func NewTestIbackupServer(t *testing.T) (*server.Server, string, string, func() error, error) { //nolint:funlen,unparam
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

	t.Setenv("XDG_STATE_HOME", filepath.Dir(certPath))

	err = s.EnableAuthWithServerToken(certPath, keyPath, ".ibackup.token",
		func(_, _ string) (bool, string) { return true, "1" })
	if err != nil {
		return nil, "", "", nil, err
	}

	if err = s.MakeQueueEndPoints(); err != nil {
		return nil, "", "", nil, err
	}

	if err = s.LoadSetDB(filepath.Join(t.TempDir(), "db"), "", ""); err != nil {
		return nil, "", "", nil, err
	}

	addr, dfn, err := gas.StartTestServer(s, certPath, keyPath)

	time.Sleep(time.Second >> 1)

	os.Setenv("XDG_STATE_HOME", "/")

	return s, addr, certPath, dfn, err
}

// NewClient returns a new ibackup client for a new server.
func NewClient(t *testing.T) *server.Client {
	t.Helper()

	_, addr, certPath, dfn, err := NewTestIbackupServer(t)
	So(err, ShouldBeNil)

	time.Sleep(time.Second >> 1)

	client, err := ibackup.Connect(addr, certPath)
	So(err, ShouldBeNil)

	Reset(func() {
		waitForSetsComplete(client)
		So(dfn(), ShouldBeNil)
	})

	return client
}

func waitForSetsComplete(client *server.Client) {
	ready := false
	for !ready {
		sets, err := client.GetSets("all")
		if err != nil {
			break
		}

		ready = true

		for _, item := range sets {
			if item.Status != set.Complete {
				ready = false

				time.Sleep(time.Millisecond * 500) //nolint:mnd

				break
			}
		}
	}
}

// NewMultiClient returns an ibackup MultiClient configured with a single
// server.
func NewMultiClient(t *testing.T) *ibackup.MultiClient { //nolint:funlen
	t.Helper()

	_, addr, certPath, dfn, err := NewTestIbackupServer(t)
	So(err, ShouldBeNil)

	time.Sleep(time.Second >> 1)

	Reset(func() {
		client, errr := ibackup.Connect(addr, certPath)
		So(errr, ShouldBeNil)

		waitForSetsComplete(client)
		So(dfn(), ShouldBeNil)
	})

	client, err := ibackup.New(ibackup.Config{
		Servers: map[string]ibackup.ServerDetails{
			"": {
				Addr:  addr,
				Cert:  certPath,
				Token: filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
			},
		},
		PathToServer: map[string]ibackup.ServerTransformer{
			"": {
				Transformer: "prefix=/:/remote/",
			},
		},
	})
	So(err, ShouldBeNil)

	return client
}
