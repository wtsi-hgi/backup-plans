/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Sky Haines <sh55@sanger.ac.uk>
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
	"net/http"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
)

type RuleStats struct {
	*db.Rule
	SizeCount
}

type DirStats struct {
	Path         string
	BackupStatus ibackup.SetBackupActivity
	RuleStats    []RuleStats
}

func (s *Server) ClaimStats(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.claimstats)
}

func (s *Server) claimstats(w http.ResponseWriter, r *http.Request) error {
	user := s.getUser(r)

	claimstats := []DirStats{}

	for _, dir := range s.directoryRules {
		if dir.ClaimedBy != user {
			continue
		}

		rulestats, err := s.generateRuleStats(dir)
		if err != nil {
			return err
		}

		sba, err := s.config.GetIBackupClient().GetBackupActivity(dir.Path, "plan::"+dir.Path, user, false)
		if err != nil {
			sba = &ibackup.SetBackupActivity{LastSuccess: time.Time{}, Name: "plan::" + dir.Path, Requester: user, Failures: 0}
		}

		claimstats = append(claimstats, DirStats{
			Path:         dir.Path,
			BackupStatus: *sba,
			RuleStats:    rulestats,
		})
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(claimstats)
}

// generateRuleStats will create a []RuleStats slice for the given directory, containing a RuleStats object for every
// rule on the directory.
func (s *Server) generateRuleStats(dir *ruletree.DirRules) ([]RuleStats, error) {
	dirSummary, err := s.rootDir.Summary(dir.Path)
	if err != nil {
		return nil, err
	}

	ids := s.gatherDirRules(dir)
	rulestats := []RuleStats{}

	for _, r := range dirSummary.RuleSummaries {
		if _, exists := ids[r.ID]; exists || r.ID == 0 {
			rulestats = append(rulestats, s.generateStatsForRule(&r))
		}
	}

	return rulestats, nil
}

func (s *Server) generateStatsForRule(r *ruletree.Rule) RuleStats {
	totalSize := uint64(0)
	totalCount := uint64(0)

	for _, stat := range r.Users {
		totalSize += stat.Size
		totalCount += stat.Files
	}

	return RuleStats{
		Rule: s.rules[r.ID],
		SizeCount: SizeCount{
			Size:  totalSize,
			Count: totalCount,
		},
	}
}

// gatherDirRules will return the ID's of all rules on the directory given.
func (s *Server) gatherDirRules(dir *ruletree.DirRules) map[uint64]struct{} {
	ids := make(map[uint64]struct{})

	for _, rule := range dir.Rules {
		ids[uint64(rule.ID())] = struct{}{} //nolint:gosec
	}

	return ids
}
