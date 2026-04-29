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
	"iter"
	"net/http"
	"slices"

	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
)

var (
	ErrNotFound      = errors.New("404 page not found")
	ErrNotAuthorised = errors.New("not authorised to see this directory")
)

type treeDB struct {
	*ruletree.DirSummary
	ClaimedBy    string
	Rules        map[string]map[uint64]rules.Rule
	Unauthorised []string
	CanClaim     bool
	rules.Directory
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

	summary, err := s.rootDir.Summary(dir)
	if err != nil {
		return err
	}

	duid, dgid := summary.IDs()
	adminGroup := s.config.GetAdminGroup()

	if !isAuthorised(summary, uid, groups, adminGroup) {
		return ErrNotAuthorised
	}

	t := treeDB{
		DirSummary:   summary,
		ClaimedBy:    summary.ClaimedBy,
		Rules:        make(map[string]map[uint64]rules.Rule),
		Unauthorised: []string{},
	}

	t.CanClaim = isOwner(uid, groups, duid, dgid)

	if directory := s.rootDir.ClaimedDirectory(dir); directory != nil {
		t.Directory = *directory
	}

	if s.rootDir.HasRules(dir) {
		t.Rules[dir] = ruleMap(s.rootDir.DirRules(dir))
	}

	for name, child := range summary.Children {
		if !isAuthorised(child, uid, groups, adminGroup) {
			t.Unauthorised = append(t.Unauthorised, name)
		}

		childPath := dir + name

		if s.rootDir.HasRules(childPath) {
			t.Rules[dir] = ruleMap(s.rootDir.DirRules(childPath))
		}
	}

	w.Header().Set("Content-Type", "application/json")

	return json.NewEncoder(w).Encode(t)
}

func ruleMap(ri iter.Seq[rules.Rule]) map[uint64]rules.Rule {
	rm := make(map[uint64]rules.Rule)

	for rule := range ri {
		rm[uint64(rule.ID)] = rule //nolint:gosec
	}

	return rm
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
