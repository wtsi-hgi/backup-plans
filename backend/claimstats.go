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
	"fmt"
	"net/http"

	"github.com/wtsi-hgi/backup-plans/db"
)

// type UserClaims struct {
// 	User        string       `json:"user"`
// 	ClaimedDirs []ClaimStats `json:"claimedDirs"`
// 	// Group       string       `json:"group"`
// }

// type ClaimStats struct {
// Path string `json:"path"`
// Sizes        map[string]*SizeCount                 `json:"sizes"`
// BackupStatus map[string]*ibackup.SetBackupActivity `json:"backupStatus"`
// Rules        []db.Rule                             `json:"rules"`
// RuleStats []RuleStats `json:"ruleStats"`
// 	RuleStats map[string][]RuleStats `json:"ruleStats"` // Dir path -> []RuleStats
// }

type ClaimStats struct {
	Path      string      `json:"path"`
	RuleStats []RuleStats `json:"ruleStats"` // Dir path -> []RuleStats
}

type RuleStats struct {
	*db.Rule
	SizeCount
}

func (s *Server) ClaimStats(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.claimstats)
}

func (s *Server) claimstats(w http.ResponseWriter, _ *http.Request) error {
	userclaims := make(map[string][]ClaimStats) // User -> []ClaimStats

	// I basically need to iterate over every dirSummary and grab user for now
	// can i get a []dirSummary or do i need to recursively traverse children?

	// for every dir
	for _, dir := range s.dirs {
		dirSummary, err := s.rootDir.Summary(dir.Path)
		if err != nil {
			return err
		}

		// build up RuleStats for each dir
		rulestats := make([]RuleStats, 0, len(dirSummary.RuleSummaries))

		for _, r := range dirSummary.RuleSummaries {
			// userStats := r.Users
			// for _, stats := range userStats {
			// 	stats.Size
			// }

			rulestats = append(rulestats, RuleStats{
				Rule:      s.rules[r.ID],
				SizeCount: SizeCount{},
			})
		}

		// then set in userclaims
		// an issue here, im getting the rules for child dirs, not claimed by userA in userA's rules
		// /lustre/scratch123/humgen/a/b/ has 2 rules,claimed by userA, these show correctly
		// but, /lustre/scratch123/humgen/a/b/newdir/ is claimed by userC, but the 2 rules on this are showing in userA's claimstats with path = /lustre/scratch123/humgen/a/b/
		userclaims[dir.ClaimedBy] = append(userclaims[dir.ClaimedBy], ClaimStats{
			Path:      dir.Path,
			RuleStats: rulestats,
		})

	}

	fmt.Println(userclaims)
	fmt.Println("dirs length:", len(s.dirs))

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(userclaims)
}
