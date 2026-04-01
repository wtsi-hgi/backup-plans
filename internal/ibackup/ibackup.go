/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *		   Sendu Bala <sb10@sanger.ac.uk>
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

package ibackup

import (
	"io"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey" //nolint:staticcheck,revive
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/baton"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/ibackup/transformer"
)

const (
	CustomTransformer     = "customTransformer"
	discoveryWaitTimeout  = 10 * time.Second
	discoveryPollInterval = 200 * time.Millisecond
)

func init() { //nolint:gochecknoinits
	transformer.Register(CustomTransformer, "^/some/path/", "/remote/path/") //nolint:errcheck
}

// NewClient returns a new ibackup client for a new server.
func NewClient(t *testing.T) *server.Client {
	t.Helper()

	_, addr, certPath, dfn, err := NewTestIbackupServer(t)
	So(err, ShouldBeNil)

	time.Sleep(time.Second >> 1)

	client, err := ibackup.Connect(addr, certPath, "")
	So(err, ShouldBeNil)

	Reset(func() {
		waitForSetDiscovery(client, discoveryWaitTimeout)
		So(dfn(), ShouldBeNil)
	})

	return client
}

// NewMultiClient returns an ibackup MultiClient configured with a single
// server.
func NewMultiClient(t *testing.T) *ibackup.MultiClient { //nolint:funlen
	t.Helper()

	_, addr, certPath, dfn, err := NewTestIbackupServer(t)
	So(err, ShouldBeNil)

	time.Sleep(time.Second >> 1)

	Reset(func() {
		client, errr := ibackup.Connect(addr, certPath, "")
		So(errr, ShouldBeNil)

		waitForSetDiscovery(client, discoveryWaitTimeout)
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

// NewTestIbackupServer returns a test ibackup server, its address, certificate
// path, a function you should defer to stop the server, and an error.
func NewTestIbackupServer(t *testing.T) (*server.Server, string, string, func() error, error) { //nolint:unparam
	t.Helper()

	if err := testirods.AddPseudoIRODsToolsToPathIfRequired(t); err != nil {
		return nil, "", "", nil, err
	}

	s, certPath, keyPath, err := newConfiguredTestServer(t)
	if err != nil {
		return nil, "", "", nil, err
	}

	addr, dfn, err := gas.StartTestServer(s, certPath, keyPath)
	if err != nil {
		return nil, "", "", nil, err
	}

	cleanup := func() error {
		client, connectErr := ibackup.Connect(addr, certPath, "")
		if connectErr == nil {
			waitForSetDiscovery(client, discoveryWaitTimeout)
		}

		return dfn()
	}

	time.Sleep(time.Second >> 1)

	return s, addr, certPath, cleanup, nil
}

func newConfiguredTestServer(t *testing.T) (*server.Server, string, string, error) {
	t.Helper()

	s, err := newTestServer()
	if err != nil {
		return nil, "", "", err
	}

	certPath, keyPath, err := configureTestServerAuth(t, s)
	if err != nil {
		return nil, "", "", err
	}

	if err = prepareTestServerSetDB(t, s); err != nil {
		return nil, "", "", err
	}

	return s, certPath, keyPath, nil
}

func newTestServer() (*server.Server, error) {
	handler, err := baton.GetBatonHandler()
	if err != nil {
		return nil, err
	}

	conf := server.Config{
		HTTPLogger:     io.Discard,
		StorageHandler: handler,
	}

	return server.New(conf)
}

func configureTestServerAuth(t *testing.T, s *server.Server) (string, string, error) {
	t.Helper()

	certPath, keyPath, err := gas.CreateTestCert(t)
	if err != nil {
		return "", "", err
	}

	t.Setenv("XDG_STATE_HOME", filepath.Dir(certPath))

	err = s.EnableAuthWithServerToken(certPath, keyPath, ".ibackup.token",
		func(_, _ string) (bool, string) { return true, "1" })
	if err != nil {
		return "", "", err
	}

	return certPath, keyPath, nil
}

func prepareTestServerSetDB(t *testing.T, s *server.Server) error {
	t.Helper()

	if err := s.MakeQueueEndPoints(); err != nil {
		return err
	}

	return s.LoadSetDB(filepath.Join(t.TempDir(), "db"), "")
}

func waitForSetDiscovery(client *server.Client, timeout time.Duration) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		inProgress, err := hasDiscoveryInProgress(client)
		if err != nil || !inProgress {
			return
		}

		time.Sleep(discoveryPollInterval)
	}
}

func hasDiscoveryInProgress(client *server.Client) (bool, error) {
	sets, err := client.GetSets("all")
	if err != nil {
		return false, err
	}

	for _, item := range sets {
		if isDiscovering(item) {
			return true, nil
		}
	}

	return false, nil
}

func isDiscovering(item *set.Set) bool {
	return !item.StartedDiscovery.IsZero() && item.LastDiscovery.Before(item.StartedDiscovery)
}
