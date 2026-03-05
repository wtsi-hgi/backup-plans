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
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
)

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

func (s *Server) claimstats(w http.ResponseWriter, r *http.Request) error {
	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	f := createClaimstatsFilter(r)
	claimstats := make([]DirStats, 0, len(s.directoryRules))
	channel := make(chan DirStats, len(s.directoryRules))

	s.collectDirStats(f, channel)

	for item := range channel {
		claimstats = append(claimstats, item)
	}

	slices.SortFunc(claimstats, func(a, b DirStats) int { return strings.Compare(a.Path, b.Path) })

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(claimstats)
}

func (s *Server) collectDirStats(f filter, channel chan DirStats) {
	var wg sync.WaitGroup

	for _, dir := range s.directoryRules {
		if !(s.matchesFilter(dir, f)) {
			continue
		}

		wg.Add(1)

		go func(dir *Directory) {
			defer wg.Done()

			// dirSummary, err := s.rootDir.Summary(dir.Path)
			// if err != nil {
			// 	return
			// }
			dirSummary := dir.DirSummary

			dirSummary.ClaimedBy = s.getClaimed(dir.Path)

			dirStats := s.generateDirStats(dir.Path, dirSummary)
			channel <- *dirStats
		}(dir)
	}

	go func() {
		wg.Wait()
		close(channel)
	}()
}

func (s *Server) matchesFilter(dir *Directory, f filter) bool {
	if !f.filterUser && !f.filterGroup {
		return false
	}

	return !s.filterOutUser(dir, f) && !s.filterOutGroupBom(dir, f)
}

// filterOutUser will return true if the user does not match the filter.
func (s *Server) filterOutUser(dir *Directory, f filter) bool {
	return f.filterUser && f.user != dir.ClaimedBy
}

// filterOutGroupBom will return true if the group/bom does not match the filter.
func (s *Server) filterOutGroupBom(dir *Directory, f filter) bool {
	return f.filterGroup && (s.dirGroups[dir.ID()] != f.group && s.dirBoms[dir.ID()] != f.group)
}

func createClaimstatsFilter(r *http.Request) filter {
	user := r.FormValue("user")
	filterUser := user != ""

	group := r.FormValue("groupbom")
	filterGroup := group != ""

	return filter{user, group, filterUser, filterGroup}
}

func (s *Server) generateDirStats(path string, dirSummary *ruletree.DirSummary) *DirStats {
	rulestats := s.generateRuleStats(path, dirSummary)

	sba := s.getBackupActivity(path, dirSummary)

	return &DirStats{
		Path:         path,
		ClaimedBy:    dirSummary.ClaimedBy,
		Group:        dirSummary.Group,
		BackupStatus: *sba,
		RuleStats:    rulestats,
	}
}

// TODO: Refactor this to dedupe code with populateBackupStatus and its subparts
func (s *Server) getBackupActivity(path string, dirSummary *ruletree.DirSummary) *ibackup.SetBackupActivity {
	for _, ruleSummary := range dirSummary.RuleSummaries {
		rule := s.rules[ruleSummary.ID]

		dirID := rule.DirID()
		if dirID <= 0 {
			continue
		}

		dirPath := s.dirs[uint64(dirID)].Path
		dir := s.directoryRules[dirPath]

		switch rule.BackupType {
		case db.BackupIBackup:
			c := s.config.GetCachedIBackupClient()

			sba, _ := c.GetBackupActivity(path, "plan::"+path, dirSummary.ClaimedBy, false) //nolint:errcheck
			if sba == nil {
				sba = &ibackup.SetBackupActivity{
					LastSuccess: time.Time{},
					Name:        "plan::" + path,
					Requester:   dirSummary.ClaimedBy,
				}
			}

			return sba
		case db.BackupManualIBackup:
			dirSet := dirSet{dir.Path, rule.Metadata}

			sba, err := s.config.GetCachedIBackupClient().GetBackupActivity(dirSet.dir, dirSet.set, dir.ClaimedBy, true)
			if err != nil {
				slog.Error("error querying manual ibackup status",
					"dir", dirSet.dir, "claimedBy", dir.ClaimedBy, "set", dirSet.set, "err", err)
			}

			if sba == nil {
				sba = &ibackup.SetBackupActivity{
					Name:      dirSet.set,
					Requester: dir.ClaimedBy,
				}
			}

			return sba
		case db.BackupManualGit:
			repo := rule.Metadata

			t, err := s.gitCache.GetLatestCommitDate(repo)
			if err != nil {
				slog.Error("error querying repo status", "repo", repo, "err", err)
			}

			return &ibackup.SetBackupActivity{
				LastSuccess: t,
				Name:        repo,
				Requester:   dir.ClaimedBy,
			}
			// default:
			// 	slog.Error("Error querying sba", dir.Path)
		}
	}
	return &ibackup.SetBackupActivity{}
}

// generateRuleStats will create a []RuleStats slice for the given directory, containing a RuleStats object for every
// rule on the directory.
func (s *Server) generateRuleStats(path string, dirSummary *ruletree.DirSummary) []ruleStats {
	ids := s.gatherDirRules(path)

	rulestats := []ruleStats{}

	for _, r := range dirSummary.RuleSummaries {
		if _, exists := ids[r.ID]; exists || r.ID == 0 {
			rulestats = append(rulestats, s.generateStatsForRule(r))
		}
	}

	return rulestats
}

func (s *Server) generateStatsForRule(r ruletree.Rule) ruleStats {
	var totalSize, totalCount uint64

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

// gatherDirRules will return the IDs of all rules on the directory given.
func (s *Server) gatherDirRules(path string) map[uint64]struct{} {
	dirRules := s.directoryRules[path].DirRules
	if dirRules == nil {
		return nil
	}

	ids := make(map[uint64]struct{})

	for _, rule := range dirRules.Rules {
		ids[uint64(rule.ID())] = struct{}{} //nolint:gosec
	}

	return ids
}
