package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
)

func (s *Server) AddTree(file string) error {
	return s.rootDir.AddTree(file)
}

type Tree struct {
	*ruletree.DirSummary
	ClaimedBy    string
	Rules        map[string]map[uint64]*db.Rule
	Unauthorised []string
}

func (s *Server) Tree(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.tree)
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	username := s.getUser(r)

	groups := users.GetGroups(username)
	if len(groups) == 0 {
		return ErrNotAuthorised
	}

	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	summary, err := s.rootDir.Summary(dir[1:])
	if err != nil {
		return err
	}

	if !isAuthorised(summary, username, groups) {
		return ErrNotAuthorised
	}

	t := Tree{
		DirSummary:   summary,
		Rules:        make(map[string]map[uint64]*db.Rule),
		Unauthorised: []string{},
	}

	for name, child := range summary.Children {
		if !isAuthorised(child, username, groups) {
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

func isAuthorised(summary *ruletree.DirSummary, username string, groups []string) bool {
	for _, rs := range summary.RuleSummaries {
		for _, u := range rs.Users {
			if u.Name == username {
				return true
			}
		}

		for _, g := range rs.Groups {
			if slices.Contains(groups, g.Name) {
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
