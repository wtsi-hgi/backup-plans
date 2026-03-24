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
	"time"

	"github.com/wtsi-hgi/backup-plans/config"
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
	BackupStatus []ibackup.SetBackupActivity
	RuleStats    []ruleStats
	LastMod      time.Time
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
	claimstats := s.collectDirStats(f)

	slices.SortFunc(claimstats, func(a, b DirStats) int { return strings.Compare(a.Path, b.Path) })

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(claimstats)
}

func (s *Server) collectDirStats(f filter) []DirStats {
	claimstats := make([]DirStats, 0, len(s.directoryRules))

	for _, dir := range s.directoryRules {
		if !(s.matchesFilter(dir, f)) {
			continue
		}

		dirSummary := dir.DirSummary
		dirSummary.ClaimedBy = s.getClaimed(dir.Path)
		claimstats = append(claimstats, *s.generateDirStats(dir, dirSummary))
	}

	return claimstats
}

func (s *Server) matchesFilter(dir *Directory, f filter) bool {
	if !f.filterUser && !f.filterGroup {
		return false
	}

	if dir.DirSummary == nil {
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

func (s *Server) generateDirStats(dir *Directory, dirSummary *ruletree.DirSummary) *DirStats {
	rulestats := s.generateRuleStats(dir.Path, dirSummary)
	sbas := s.gatherSBAs(dir, dirSummary)
	mod, _ := s.getLastMod(dir.Path) //nolint:errcheck

	return &DirStats{
		Path:         dir.Path,
		ClaimedBy:    dirSummary.ClaimedBy,
		Group:        dirSummary.Group,
		BackupStatus: sbas,
		RuleStats:    rulestats,
		LastMod:      mod,
	}
}

func (s *Server) gatherSBAs(dir *Directory, dirSummary *ruletree.DirSummary) []ibackup.SetBackupActivity {
	sbas := make([]ibackup.SetBackupActivity, 0, len(dirSummary.RuleSummaries))
	seen := make(map[string]struct{})

	for _, ruleSummary := range dirSummary.RuleSummaries {
		rule := s.rules[ruleSummary.ID]

		rdirID := rule.DirID()
		if rdirID <= 0 || dir.ID() != rdirID {
			continue
		}

		sbas = s.addSBA(sbas, seen, dir, rule)
	}

	return sbas
}

// addSBA will retrieve the ibackup.SetBackupActivity for a given set and add it to sbas. Duplicates are skipped.
func (s *Server) addSBA( //nolint:gocognit,gocyclo,funlen
	sbas []ibackup.SetBackupActivity,
	seen map[string]struct{},
	dir *Directory,
	rule *db.Rule,
) []ibackup.SetBackupActivity {
	requester := dir.ClaimedBy

	switch rule.BackupType { //nolint:exhaustive
	case db.BackupIBackup:
		backupName := "plan::" + dir.Path
		if _, exists := seen[backupName]; !exists {
			sbas = append(sbas, s.getIBackupBackupStatus(backupName, dir.Path, requester))
			seen[backupName] = struct{}{}
		}

	case db.BackupManualIBackup:
		dirSet := dirSet{dir.Path, rule.Metadata}
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
		sba, err := s.getNFSStatus(rule.Metadata, requester)
		if _, exists := seen[rule.Metadata]; !exists && err == nil {
			sbas = append(sbas, sba)
			seen[rule.Metadata] = struct{}{}
		}
	}

	return sbas
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

func (s *Server) getLastMod(path string) (time.Time, error) {
	client := s.config.GetWRStatClient()
	if client == config.NullWRStat {
		return time.Time{}, ErrNoClient
	}

	if path == "" {
		return time.Time{}, ErrInvalidDir
	}

	m, err := client.GetWRStatModTime(path)
	if err != nil {
		slog.Error("error querying wrstat status", "path", path, "err", err)
	}

	return m, nil
}
