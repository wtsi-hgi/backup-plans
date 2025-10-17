/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
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

package backend

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/httpbuffer"

	_ "vimagination.zapto.org/httpbuffer/gzip"
)

// Server represents all of the data required to run the backend server.
type Server struct {
	getUser func(r *http.Request) string

	rulesMu        sync.RWMutex
	rulesDB        *db.DB
	directoryRules map[string]*ruletree.DirRules
	dirs           map[uint64]*db.Directory
	rules          map[uint64]*db.Rule
	stateMachine   group.StateMachine[db.Rule]
	reportRoots    []string
	adminGroup     uint32

	rootDir *ruletree.RootDir
}

// New creates a new Backend API server.
func New(db *db.DB, getUser func(r *http.Request) string, reportRoots []string) (*Server, error) {
	s := &Server{
		getUser:     getUser,
		rulesDB:     db,
		reportRoots: reportRoots,
	}

	rules, err := s.loadRules()
	if err != nil {
		return nil, err
	}

	s.rootDir, err = ruletree.NewRoot(rules)
	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Server) SetAdminGroup(gid uint32) {
	s.adminGroup = gid
}

// WhoAmI is an HTTP endpoint that returns the result of the getUser func that
// was passed to the New function.
func (s *Server) WhoAmI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(s.getUser(r))
}

func handle(w http.ResponseWriter, r *http.Request, fn func(http.ResponseWriter, *http.Request) error) {
	httpbuffer.Handler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := fn(w, r); err != nil {
				var errc Error

				if errors.As(err, &errc) {
					http.Error(w, errc.Err.Error(), errc.Code)

					return
				}

				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}),
	}.ServeHTTP(w, r)
}

// Error is an error that contains an HTTP error code.
type Error struct {
	Code int
	Err  error
}

func (e Error) Error() string {
	return e.Err.Error()
}
