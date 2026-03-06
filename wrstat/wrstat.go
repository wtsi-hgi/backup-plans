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
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/wtsi-hgi/activecache"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/wrstat-ui/server"
)

// Config contains the required settings to connect to a WRStat server.
type Config struct {
	JWTBasename, ServerTokenBasename, ServerURL, ServerCert, Username string
	OktaMode                                                          bool
}

// Client is a WRStat client with cached responses.
type Client struct {
	mu     sync.RWMutex
	client *gas.ClientCLI
	cache  *activecache.Cache[string, time.Time]
}

// New creates a new WRStat Client from the given config.
func New(d time.Duration, cfg Config) (*Client, error) {
	var username []string

	if cfg.Username != "" {
		username = append(username, cfg.Username)
	}

	client, err := gas.NewClientCLI(
		cfg.JWTBasename, cfg.ServerTokenBasename,
		cfg.ServerURL, cfg.ServerCert, cfg.OktaMode, username...,
	)
	if err != nil {
		return nil, err
	}

	c := &Client{client: client}

	c.cache = activecache.New(d, c.getWRStatModTime)

	return c, nil
}

// UpdateConfig updates the WRStat client to use the new config specified.
func (c *Client) UpdateConfig(cfg Config) error {
	client, err := gas.NewClientCLI(
		cfg.JWTBasename, cfg.ServerTokenBasename,
		cfg.ServerURL, cfg.ServerCert, cfg.OktaMode,
	)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.client = client
	c.mu.Unlock()

	return nil
}

func (c *Client) getWRStatModTime(path string) (time.Time, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	_, dss, err := server.GetWhereDataIs(client, path, "", "", "", 0, "0")
	if err != nil {
		return time.Time{}, err
	}

	if len(dss) == 0 {
		return time.Time{}, fs.ErrNotExist
	}

	return dss[0].Mtime, nil
}

// GetWRStatModTime queries the configured WRStat server to retrieve the latest
// mtime for the given path.
func (c *Client) GetWRStatModTime(path string) (time.Time, error) {
	return c.cache.Get(filepath.Clean(path))
}

// Stop stops the concurrent retrieval of mtimes.
func (c *Client) Stop() {
	c.cache.Stop()
}
