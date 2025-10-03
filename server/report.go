package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"vimagination.zapto.org/tree"
)

type summary struct {
	Summaries map[string]*ruletree.DirSummary
	Rules     map[uint64]*db.Rule
}

func (s *Server) Summary(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.summary)
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) error {
	summary := summary{
		Summaries: make(map[string]*ruletree.DirSummary, len(s.reportRoots)),
		Rules:     make(map[uint64]*db.Rule),
	}

	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	for _, root := range s.reportRoots {
		ds, err := s.rootDir.Summary(root[1:])
		if errors.Is(err, ruletree.ErrNotFound) || errors.As(err, new(tree.ChildNotFoundError)) {
			continue
		} else if err != nil {
			return err
		}

		summary.Summaries[root] = ds

		for _, rule := range ds.RuleSummaries {
			if _, ok := summary.Rules[rule.ID]; ok {
				continue
			}

			summary.Rules[rule.ID] = s.rules[rule.ID]
		}
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(summary)
}
