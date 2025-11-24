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
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
	"vimagination.zapto.org/httpbuffer"
	_ "vimagination.zapto.org/httpbuffer/gzip" //
)

// Server represents all of the data required to run the backend server.
type Server struct {
	getUser func(r *http.Request) string

	rulesMu        sync.RWMutex
	rulesDB        *db.DB
	directoryRules map[string]*ruletree.DirRules
	dirs           map[uint64]*db.Directory
	rules          map[uint64]*db.Rule
	reportRoots    []string
	adminGroup     uint32
	cache          *ibackup.MultiCache
	owners         map[string][]string
	bom            map[string][]string

	rootDir *ruletree.RootDir
}

// New creates a new Backend API server.
func New(db *db.DB, getUser func(r *http.Request) string, reportRoots []string,
	ibackupclient *ibackup.MultiClient, owners, bom string) (*Server, error) {
	s := &Server{
		getUser:     getUser,
		rulesDB:     db,
		reportRoots: reportRoots,
		cache:       ibackup.NewMultiCache(ibackupclient, time.Hour),
	}

	if err := s.loadOwners(owners); err != nil {
		return nil, err
	}

	if err := s.loadBOM(bom); err != nil {
		return nil, err
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

func (s *Server) loadOwners(owners string) error {
	if owners == "" {
		return nil
	}

	f, err := os.Open(owners)
	if err != nil {
		return err
	}

	defer f.Close()

	ownersMap := make(map[string][]string)

	r := csv.NewReader(f)

	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		} else if len(record) < 2 {
			continue
		}

		gid, err := strconv.ParseUint(record[0], 10, 32)
		if err != nil {
			return err
		}

		ownersMap[record[1]] = append(ownersMap[record[1]], users.Group(uint32(gid)))
	}

	s.rulesMu.Lock()
	s.owners = ownersMap
	s.rulesMu.Unlock()

	return nil
}

func (s *Server) loadBOM(bom string) error {
	if bom == "" {
		return nil
	}

	f, err := os.Open(bom)
	if err != nil {
		return err
	}

	defer f.Close()

	bomMap := make(map[string][]string)

	r := csv.NewReader(f)

	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		} else if len(record) < 2 {
			continue
		}

		bomMap[record[1]] = append(bomMap[record[1]], record[0])
	}

	s.rulesMu.Lock()
	s.bom = bomMap
	s.rulesMu.Unlock()

	return nil
}

func (s *Server) SetAdminGroup(gid uint32) {
	s.adminGroup = gid
}

// WhoAmI is an HTTP endpoint that returns the result of the getUser func that
// was passed to the New function.
func (s *Server) WhoAmI(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(s.getUser(r)) //nolint:errcheck,errchkjson
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

// SetExists is an HTTP endpoint that will return whether there is an ibackup
// set with:
//
//	Set name: The requests metadata form value.
//		User: Given by the getUser func passed when creating the server with New(...)
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

	got, err := s.cache.GetBackupActivity(dir, setName, user)
	if err != nil {
		if err.Error() == "set with that id does not exist" {
			return nil
		}

		return err
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(got != nil)
}
