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
	"slices"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/backup-plans/ruletree"
)

type ruleStats struct {
	rules.Rule
	SizeCount
}

// DirStats holds information about a claimed directory and its rules.
type DirStats struct {
	Path         string
	ClaimedBy    string
	Group        string
	BackupStatus []ibackup.SetBackupActivity
	RuleStats    []ruleStats
	LastMod      uint64
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
	f := createClaimstatsFilter(r)
	claimstats := s.collectDirStats(f)

	slices.SortFunc(claimstats, func(a, b DirStats) int { return strings.Compare(a.Path, b.Path) })

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(claimstats)
}

func (s *Server) collectDirStats(f filter) []DirStats {
	claimstats := make([]DirStats, 0)

	for dir, summary := range s.rootDir.ClaimedSummaries() {
		if !s.matchesFilter(summary, f) {
			continue
		}

		claimstats = append(claimstats, *s.generateDirStats(dir, summary))
	}

	return claimstats
}

func (s *Server) matchesFilter(dir *ruletree.DirSummary, f filter) bool {
	if !f.filterUser && !f.filterGroup {
		return false
	}

	if dir == nil {
		return false
	}

	return !s.filterOutUser(dir, f) && !s.filterOutGroupBom(dir, f)
}

// filterOutUser will return true if the user does not match the filter.
func (s *Server) filterOutUser(dir *ruletree.DirSummary, f filter) bool {
	return f.filterUser && f.user != dir.ClaimedBy
}

// filterOutGroupBom will return true if the group/bom does not match the filter.
func (s *Server) filterOutGroupBom(dir *ruletree.DirSummary, f filter) bool {
	return f.filterGroup && (dir.Group != f.group && s.groupBOM(dir.Group) != f.group)
}

func (s *Server) groupBOM(group string) string {
	bom, _ := s.groupBOMs.Get(group) //nolint:errcheck

	return bom
}

func createClaimstatsFilter(r *http.Request) filter {
	user := r.FormValue("user")
	filterUser := user != ""

	group := r.FormValue("groupbom")
	filterGroup := group != ""

	return filter{user, group, filterUser, filterGroup}
}

func (s *Server) generateDirStats(dir string, dirSummary *ruletree.DirSummary) *DirStats {
	rulestats := s.generateRuleStats(dir, dirSummary)
	sbas := s.gatherSBAs(dir, dirSummary)

	return &DirStats{
		Path:         dir,
		ClaimedBy:    dirSummary.ClaimedBy,
		Group:        dirSummary.Group,
		BackupStatus: sbas,
		RuleStats:    rulestats,
		LastMod:      dirSummary.LastMod,
	}
}

func (s *Server) gatherSBAs(dir string, dirSummary *ruletree.DirSummary) []ibackup.SetBackupActivity {
	sbas := make([]ibackup.SetBackupActivity, 0, len(dirSummary.RuleSummaries))
	seen := make(map[string]struct{})

	for rule := range s.rootDir.DirRules(dir) {
		sbas = s.addSBA(sbas, seen, dir, dirSummary.ClaimedBy, rule)
	}

	return sbas
}

// addSBA will retrieve the ibackup.SetBackupActivity for a given set and add it to sbas. Duplicates are skipped.
func (s *Server) addSBA( //nolint:gocyclo,funlen
	sbas []ibackup.SetBackupActivity,
	seen map[string]struct{},
	dir, requester string,
	rule rules.Rule,
) []ibackup.SetBackupActivity {
	switch rule.BackupType {
	case db.BackupIBackup:
		backupName := "plan::" + dir
		if _, exists := seen[backupName]; !exists {
			sbas = append(sbas, s.getIBackupBackupStatus(backupName, dir, requester))
			seen[backupName] = struct{}{}
		}

	case db.BackupManualIBackup:
		dirSet := dirSet{dir, rule.Metadata}
		if _, exists := seen[rule.Metadata]; !exists {
			sbas = append(sbas, s.getManualIBackupStatus(dirSet, requester))
			seen[rule.Metadata] = struct{}{}
		}

	case db.BackupManualGit:
		if _, exists := seen[rule.Metadata]; !exists {
			sbas = append(sbas, s.getGitBackupStatus(rule.Metadata, requester))
			seen[rule.Metadata] = struct{}{}
		}

	case db.BackupManualNFS:
		sba := s.getNFSStatus(rule.Metadata, requester)
		if _, exists := seen[rule.Metadata]; !exists {
			sbas = append(sbas, sba)
			seen[rule.Metadata] = struct{}{}
		}
	}

	return sbas
}

// generateRuleStats will create a []RuleStats slice for the given directory, containing a RuleStats object for every
// rule on the directory.
func (s *Server) generateRuleStats(path string, dirSummary *ruletree.DirSummary) []ruleStats {
	ruleList := slices.Collect(s.rootDir.DirRules(path))
	ids := make(map[uint64]rules.Rule, len(ruleList))

	for _, rule := range ruleList {
		ids[uint64(rule.ID)] = rule //nolint:gosec
	}

	rulestats := []ruleStats{}

	for _, r := range dirSummary.RuleSummaries {
		if rule, exists := ids[r.ID]; exists || r.ID == 0 {
			rulestats = append(rulestats, s.generateStatsForRule(r, rule))
		}
	}

	return rulestats
}

func (s *Server) generateStatsForRule(r ruletree.Rule, rule rules.Rule) ruleStats {
	var totalSize, totalCount uint64

	for _, stat := range r.Users {
		totalSize += stat.Size
		totalCount += stat.Files
	}

	return ruleStats{
		Rule: rule,
		SizeCount: SizeCount{
			Size:  totalSize,
			Count: totalCount,
		},
	}
}
