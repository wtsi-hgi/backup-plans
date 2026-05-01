/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *		   Sky Haines <sh55@sanger.ac.uk>
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
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/wtsi-hgi/activecache"
	"github.com/wtsi-hgi/backup-plans/config"
	"github.com/wtsi-hgi/backup-plans/git"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"vimagination.zapto.org/httpbuffer"
	_ "vimagination.zapto.org/httpbuffer/gzip" //
)

// Server represents all of the data required to run the backend server.
type Server struct {
	getUser func(r *http.Request) string

	config   *config.Config
	gitCache *git.Cache

	rootDir   *ruletree.RootDir
	groupBOMs *activecache.Cache[string, string]

	exit func()
}

// New creates a new Backend API server.
func New(root *ruletree.RootDir, getUser func(r *http.Request) string, c *config.Config) *Server {
	s := &Server{
		getUser: getUser,
		config:  c,
		rootDir: root,
		groupBOMs: activecache.New(time.Hour, func(group string) (string, error) {
			for bom, groups := range c.GetBOMs() {
				if slices.Contains(groups, group) {
					return bom, nil
				}
			}

			return "", nil
		}),
	}

	s.gitCache = git.NewCache(time.Hour)

	ctx, done := context.WithCancel(context.Background())

	go s.refreezer(ctx)

	s.exit = func() {
		s.groupBOMs.Stop()
		done()
	}

	return s
}

// WhoAmI is an HTTP endpoint that returns the result of the getUser func that
// was passed to the New function.
func (s *Server) WhoAmI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(s.getUser(r)) //nolint:errcheck,errchkjson
}

func handle(w http.ResponseWriter, r *http.Request, fn func(http.ResponseWriter, *http.Request) error) {
	httpbuffer.Handler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := fn(w, r); err != nil { //nolint:nestif
				code := http.StatusInternalServerError

				if c, ok := httpErrors[err]; ok {
					code = c
				} else if numErr := new(strconv.NumError); errors.As(err, &numErr) {
					code = http.StatusBadRequest
				}

				http.Error(w, err.Error(), code)
			}
		}),
	}.ServeHTTP(w, r)
}

// SetExists is an HTTP endpoint that will return whether there is a manual
// ibackup set with:
//
//	Set name: The requests metadata form value.
//	User: Given by the getUser func passed when creating the server with New(...)
func (s *Server) SetExists(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.setExists)
}

func (s *Server) setExists(w http.ResponseWriter, r *http.Request) error {
	user := s.getUser(r)
	setName := r.FormValue("metadata")

	dir, err := getDir(r)
	if err != nil {
		return err
	}

	client := s.config.GetCachedIBackupClient()
	if client == nil {
		return ErrNoIBackup
	}

	got, err := client.GetBackupActivity(dir, setName, user, true)
	if err != nil {
		if err.Error() != "set with that id does not exist" {
			return err
		}
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(got != nil)
}

func (s *Server) GetMainProgrammes(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.getMainProgrammes)
}

func (s *Server) getMainProgrammes(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(s.config.GetMainProgrammes())
}

func (s *Server) refreezer(ctx context.Context) {
	for {
		select {
		case <-time.After(10 * time.Minute): //nolint:mnd
		case <-ctx.Done():
			return
		}

		s.refreezeUpdatedDirectories()
	}
}

func (s *Server) refreezeUpdatedDirectories() {
	client := s.config.GetCachedIBackupClient()

	for _, dir := range slices.Collect(s.rootDir.ClaimedDirectories()) {
		if dir.Melt == 0 {
			continue
		}

		ba, err := client.GetBackupActivity(dir.Path, setNamePrefix+dir.Path, dir.ClaimedBy, false)
		if err != nil || !ba.LastSuccess.After(time.Unix(dir.Melt, 0)) {
			continue
		}

		if err := s.rootDir.Refreeze(dir.Path); err != nil {
			slog.Error("error refreezing directory", "path", dir.Path, "err", err)
		}
	}
}
