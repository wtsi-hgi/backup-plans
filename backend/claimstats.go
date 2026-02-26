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
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
)

var ErrNoFilter = errors.New("must provide a user, group or BOM to filter by")

type ruleStats struct {
	*db.Rule
	SizeCount
}

// DirStats holds information about a claimed directory and its rules.
type DirStats struct {
	Path         string
	ClaimedBy    string
	Group        string
	BackupStatus ibackup.SetBackupActivity
	RuleStats    []ruleStats
}

type filter struct {
	user        string
	group       string
	filterUser  bool
	filterGroup bool
}

// ClaimStats is an HTTP endpoint that produces a DirStats summary for every claimed directory.
func (s *Server) ClaimStats(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.claimstats)
}

func (s *Server) claimstats(w http.ResponseWriter, r *http.Request) error { //nolint:funlen
	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	f := s.getFormValues(r)
	claimstats := make([]DirStats, 0, len(s.directoryRules))
	channel := make(chan DirStats, len(s.directoryRules))

	var wg sync.WaitGroup

	for _, dir := range s.directoryRules {
		if !(s.matchesFilter(dir, f)) {
			continue
		}

		wg.Add(1)

		go func(dir *ruletree.DirRules) {
			defer wg.Done()

			dirSummary, err := s.rootDir.Summary(dir.Path)
			if err != nil {
				return
			}

			dirSummary.ClaimedBy = s.getClaimed(dir.Path)

			dirStats := s.generateDirStats(dir.Path, dirSummary)
			channel <- *dirStats
		}(dir)
	}

	go func() {
		wg.Wait()
		close(channel)
	}()

	for item := range channel {
		claimstats = append(claimstats, item)
	}

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(claimstats)
}

func (s *Server) matchesFilter(dir *ruletree.DirRules, f filter) bool {
	if !f.filterUser && !f.filterGroup {
		return false
	}

	if s.filterOutUser(dir, f) || s.filterOutGroupBom(dir, f) {
		return false
	}

	return true
}

// filterOutUser will return true if the user does not match the filter.
func (s *Server) filterOutUser(dir *ruletree.DirRules, f filter) bool {
	return dir.ClaimedBy == "" || (f.filterUser && f.user != dir.ClaimedBy)
}

// filterGroup will return true if the group/bom does not match the filter.
func (s *Server) filterOutGroupBom(dir *ruletree.DirRules, f filter) bool {
	return f.filterGroup && (s.dirGroups[dir.ID()] != f.group && s.dirBoms[dir.ID()] != f.group)
}

func (s *Server) getFormValues(r *http.Request) filter {
	filterUser := false
	filterGroup := false

	user := r.FormValue("user")
	if user != "" {
		filterUser = true
	}

	group := r.FormValue("groupbom")
	if group != "" {
		filterGroup = true
	}

	return filter{user, group, filterUser, filterGroup}
}

func (s *Server) generateDirStats(path string, dirSummary *ruletree.DirSummary) *DirStats {
	rulestats := s.generateRuleStats(path, dirSummary)

	sba, _ := s.config.GetCachedIBackupClient().GetBackupActivity(path, "plan::"+path, dirSummary.ClaimedBy, false)
	if sba == nil {
		sba = &ibackup.SetBackupActivity{
			LastSuccess: time.Time{},
			Name:        "plan::" + path,
			Requester:   dirSummary.ClaimedBy,
		}
	}

	return &DirStats{
		Path:         path,
		ClaimedBy:    dirSummary.ClaimedBy,
		Group:        dirSummary.Group,
		BackupStatus: *sba,
		RuleStats:    rulestats,
	}
}

// generateRuleStats will create a []RuleStats slice for the given directory, containing a RuleStats object for every
// rule on the directory.
func (s *Server) generateRuleStats(path string, dirSummary *ruletree.DirSummary) []ruleStats {
	ids := s.gatherDirRules(path)

	rulestats := []ruleStats{}

	for _, r := range dirSummary.RuleSummaries {
		if _, exists := ids[r.ID]; exists || r.ID == 0 {
			rulestats = append(rulestats, s.generateStatsForRule(&r))
		}
	}

	return rulestats
}

func (s *Server) generateStatsForRule(r *ruletree.Rule) ruleStats {
	totalSize := uint64(0)
	totalCount := uint64(0)

	for _, stat := range r.Users {
		totalSize += stat.Size
		totalCount += stat.Files
	}

	return ruleStats{
		Rule: s.rules[r.ID],
		SizeCount: SizeCount{
			Size:  totalSize,
			Count: totalCount,
		},
	}
}

// gatherDirRules will return the ID's of all rules on the directory given.
func (s *Server) gatherDirRules(path string) map[uint64]struct{} {
	dirRules := s.directoryRules[path]
	if dirRules == nil {
		return nil
	}

	ids := make(map[uint64]struct{})

	for _, rule := range dirRules.Rules {
		ids[uint64(rule.ID())] = struct{}{} //nolint:gosec
	}

	return ids
}
