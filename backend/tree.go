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
}

// Tree is an HTTP endpoint that returns data about a given directory and its
// direct children.
func (s *Server) Tree(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.tree)
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) error {
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

	summary, err := s.rootDir.Summary(dir[1:])
	if err != nil {
		return err
	}

	duid, dgid := summary.IDs()

	if !isAuthorised(summary, uid, groups) {
		return ErrNotAuthorised
	}

	t := treeDB{
		DirSummary:   summary,
		Rules:        make(map[string]map[uint64]*db.Rule),
		Unauthorised: []string{},
	}

	t.CanClaim = uid == duid || slices.Contains(groups, dgid)

	for name, child := range summary.Children {
		if !isAuthorised(child, uid, groups) {
			t.Unauthorised = append(t.Unauthorised, name)
		}
	}

	dirRules, ok := s.directoryRules[dir]
	if ok {
		t.ClaimedBy = dirRules.ClaimedBy
		thisDir := make(map[uint64]*db.Rule)
		t.Rules[dir] = thisDir

		for _, rule := range dirRules.Rules {
			thisDir[uint64(rule.ID())] = rule
		}
	}

	for _, rs := range t.RuleSummaries {
		if rs.ID == 0 {
			continue
		}

		rule := s.rules[rs.ID]
		dir := s.dirs[uint64(rule.DirID())]

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

func isAuthorised(summary *ruletree.DirSummary, uid uint32, groups []uint32) bool {
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

var (
	ErrNotFound = Error{
		Code: http.StatusNotFound,
		Err:  errors.New("404 page not found"),
	}
	ErrNotAuthorised = Error{
		Code: http.StatusUnauthorized,
		Err:  errors.New("not authorised to see this directory"),
	}
)
