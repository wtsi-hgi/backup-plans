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

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ruletree"
)

type RuleStats struct {
	*db.Rule
	SizeCount
}

func (s *Server) ClaimStats(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.claimstats)
}

func (s *Server) claimstats(w http.ResponseWriter, _ *http.Request) error {
	userclaims := make(map[string]map[string][]RuleStats) // User -> Dirpath -> []RuleStats

	for _, dir := range s.dirs {
		dirSummary, err := s.rootDir.Summary(dir.Path)
		if err != nil {
			return err
		}

		for _, r := range dirSummary.RuleSummaries {
			rulestats := s.generateRuleStats(&r)

			if _, exists := userclaims[dir.ClaimedBy]; !exists {
				userclaims[dir.ClaimedBy] = make(map[string][]RuleStats)
			}

			userclaims[dir.ClaimedBy][dir.Path] = append(userclaims[dir.ClaimedBy][dir.Path], rulestats)
		}
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(userclaims)
}

func (s *Server) generateRuleStats(r *ruletree.Rule) RuleStats {
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
