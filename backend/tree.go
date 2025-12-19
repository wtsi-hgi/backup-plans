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
	"slices"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
)

var (
	ErrNotFound = Error{
		Code: http.StatusNotFound,
		Err:  errors.New("404 page not found"), //nolint:err113
	}
	ErrNotAuthorised = Error{
		Code: http.StatusUnauthorized,
		Err:  errors.New("not authorised to see this directory"), //nolint:err113
	}
)

// AddTree adds a tree database, specified by the given file path, to the
// server, possibly overriding an existing database if they share the same root.
func (s *Server) AddTree(file string) error {
	return s.rootDir.AddTree(file)
}

type treeDB struct {
	*ruletree.DirSummary
	ClaimedBy    string
	Rules        map[string]map[uint64]*db.Rule
	Unauthorised []string
	CanClaim     bool
	dirDetails
}

// Tree is an HTTP endpoint that returns data about a given directory and its
// direct children.
func (s *Server) Tree(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.tree)
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) error { //nolint:funlen,gocyclo,cyclop,gocognit
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	uid, groups := users.GetIDs(s.getUser(r))
	if len(groups) == 0 {
		return ErrNotAuthorised
	}

	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	summary, err := s.rootDir.Summary(dir)
	if err != nil {
		return err
	}

	duid, dgid := summary.IDs()

	if !isAuthorised(summary, uid, groups, s.adminGroup) {
		return ErrNotAuthorised
	}

	t := treeDB{
		DirSummary:   summary,
		Rules:        make(map[string]map[uint64]*db.Rule),
		Unauthorised: []string{},
	}

	t.CanClaim = isOwner(uid, groups, duid, dgid)

	for name, child := range summary.Children {
		if !isAuthorised(child, uid, groups, s.adminGroup) {
			t.Unauthorised = append(t.Unauthorised, name)
		}
	}

	dirRules, ok := s.directoryRules[dir]
	if ok {
		t.ClaimedBy = dirRules.ClaimedBy
		thisDir := make(map[uint64]*db.Rule)
		t.Rules[dir] = thisDir

		t.dirDetails = dirDetails{
			Frequency:  dirRules.Frequency,
			ReviewDate: dirRules.ReviewDate,
			RemoveDate: dirRules.RemoveDate,
		}

		for _, rule := range dirRules.Rules {
			thisDir[uint64(rule.ID())] = rule //nolint:gosec
		}
	}

	for _, rs := range t.RuleSummaries {
		if rs.ID == 0 {
			continue
		}

		rule := s.rules[rs.ID]
		dir := s.dirs[uint64(rule.DirID())] //nolint:gosec

		r, ok := t.Rules[dir.Path]
		if !ok {
			r = make(map[uint64]*db.Rule)
			t.Rules[dir.Path] = r
		}

		r[rs.ID] = rule
	}

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(t)
}

func isOwner(uid uint32, groups []uint32, duid, dgid uint32) bool {
	return duid == uid || slices.Contains(groups, dgid)
}

func isAuthorised(summary *ruletree.DirSummary, uid uint32, groups []uint32, adminGID uint32) bool { //nolint:gocyclo,gocognit,lll
	duid, dgid := summary.IDs()
	if isOwner(uid, groups, duid, dgid) || slices.Contains(groups, adminGID) {
		return true
	}

	for _, rs := range summary.RuleSummaries {
		for _, u := range rs.Users {
			if u.ID() == uid {
				return true
			}
		}

		for _, g := range rs.Groups {
			if slices.Contains(groups, g.ID()) {
				return true
			}
		}
	}

	return false
}
