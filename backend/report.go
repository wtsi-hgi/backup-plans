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
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
	"vimagination.zapto.org/tree"
)

type summary struct {
	Summaries             map[string]*ruletree.DirSummary
	Rules                 map[uint64]*db.Rule
	Directories           map[string][]uint64
	BackupStatus          map[string]*ibackup.SetBackupActivity
	GroupBackupTypeTotals map[string]map[int]*SizeCount
}

const (
	unplanned     = -1
	setNamePrefix = "plan::"
)

type SizeCount struct {
	Count uint64 `json:"count"`
	Size  uint64 `json:"size"`
}

type dirSet struct {
	dir, set string
}

func (s *Server) addTotals(backupType int, group ruletree.Stats, summary *summary) {
	groupTotals, ok := summary.GroupBackupTypeTotals[group.Name]
	if !ok {
		groupTotals = make(map[int]*SizeCount)
		summary.GroupBackupTypeTotals[group.Name] = groupTotals
	}

	counts, ok := groupTotals[backupType]
	if !ok {
		counts = new(SizeCount)
		groupTotals[backupType] = counts
	}

	counts.Count += group.Files
	counts.Size += group.Size
}

// Summary is an HTTP endpoint that produces a backup summary of all the
// directories that were passed as reporting roots to the New function.
func (s *Server) Summary(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.summary)
}

func (s *Server) summary(w http.ResponseWriter, _ *http.Request) error {
	reportingRoots := s.rootDir.GlobPaths(s.config.GetReportingRoots()...)

	dirSummary := summary{
		Summaries:             make(map[string]*ruletree.DirSummary, len(reportingRoots)),
		Rules:                 make(map[uint64]*db.Rule),
		Directories:           make(map[string][]uint64),
		BackupStatus:          make(map[string]*ibackup.SetBackupActivity),
		GroupBackupTypeTotals: make(map[string]map[int]*SizeCount),
	}

	s.rulesMu.RLock()
	defer s.rulesMu.RUnlock()

	err := s.collectBackupTotals(&dirSummary)
	if err != nil {
		return err
	}

	s.buildRootDirSummary(reportingRoots, &dirSummary)

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(dirSummary)
}

func (s *Server) getClaimed(root string) string {
	dirRules, ok := s.directoryRules[root]
	if ok {
		return dirRules.ClaimedBy
	}

	return ""
}

func (s *Server) populateBackupStatus(dirClaims, repos, nfs map[string]string,
	manualIbackup map[string][]dirSet, dirSummary *summary,
) {
	s.populateIbackupStatus(dirClaims, dirSummary)
	s.populateManualIBackupStatus(manualIbackup, dirSummary)
	s.populateGitBackupStatus(repos, dirSummary)
	s.populateNFSStatus(nfs, dirSummary)
}

func (s *Server) populateIbackupStatus(dirClaims map[string]string, dirSummary *summary) {
	for dir, claimedBy := range dirClaims {
		planName := setNamePrefix + dir
		dirSummary.BackupStatus[dir] = s.getIBackupBackupStatus(planName, dir, claimedBy)
	}
}

func (s *Server) getIBackupBackupStatus(planName, dir, claimedBy string) *ibackup.SetBackupActivity {
	sba, err := s.config.GetCachedIBackupClient().GetBackupActivity(dir, planName, claimedBy, false)
	if err != nil {
		slog.Error("error querying ibackup status", "dir", dir, "err", err)
	}

	if sba == nil {
		sba = &ibackup.SetBackupActivity{
			Name:      planName,
			Requester: claimedBy,
		}
	}

	return sba
}

func (s *Server) populateManualIBackupStatus(manualIbackup map[string][]dirSet, dirSummary *summary) {
	for claimedBy, dirSets := range manualIbackup {
		for _, dirSet := range dirSets {
			sba := s.getManualIBackupStatus(dirSet, claimedBy)
			dirSummary.BackupStatus[claimedBy+":"+dirSet.set] = sba
		}
	}
}

func (s *Server) getManualIBackupStatus(dirSet dirSet, claimedBy string) *ibackup.SetBackupActivity {
	sba, err := s.config.GetCachedIBackupClient().GetBackupActivity(dirSet.dir, dirSet.set, claimedBy, true)
	if err != nil {
		slog.Error("error querying manual ibackup status",
			"dir", dirSet.dir, "claimedBy", claimedBy, "set", dirSet.set, "err", err)
	}

	if sba == nil {
		sba = &ibackup.SetBackupActivity{
			Name:      dirSet.set,
			Requester: claimedBy,
		}
	}

	return sba
}

func (s *Server) populateGitBackupStatus(repos map[string]string, dirSummary *summary) {
	for repo, claimedBy := range repos {
		dirSummary.BackupStatus[repo] = s.getGitBackupStatus(repo, claimedBy)
	}
}

func (s *Server) getGitBackupStatus(repo, claimedBy string) *ibackup.SetBackupActivity {
	t, err := s.gitCache.GetLatestCommitDate(repo)
	if err != nil {
		slog.Error("error querying repo status", "repo", repo, "err", err)
	}

	return &ibackup.SetBackupActivity{
		LastSuccess: t,
		Name:        repo,
		Requester:   claimedBy,
		Failures:    -1,
	}
}

func (s *Server) populateNFSStatus(paths map[string]string, dirSummary *summary) {
	client := s.config.GetWRStatClient()
	if client == nil {
		return
	}

	for path, claimedBy := range paths {
		t, err := client.GetWRStatModTime(path)
		if err != nil {
			slog.Error("error querying wrstat status", "path", path, "err", err)
		}

		dirSummary.BackupStatus[path] = &ibackup.SetBackupActivity{
			LastSuccess: t,
			Name:        path,
			Requester:   claimedBy,
			Failures:    -1,
		}
	}
}

func (s *Server) collectBackupTotals(dirSummary *summary) error {
	ds, err := s.rootDir.Summary("/")
	if err != nil {
		return err
	}

	for _, summary := range ds.RuleSummaries {
		for _, group := range summary.Groups {
			bType := s.getBackupTypeForTotals(summary.ID)
			s.addTotals(bType, group, dirSummary)
		}
	}

	return nil
}

func (s *Server) getBackupTypeForTotals(id uint64) int {
	if id == 0 {
		return unplanned
	}

	return int(s.rules[id].BackupType)
}

func (s *Server) buildRootDirSummary(reportingRoots []string, dirSummary *summary) {
	dirClaims := make(map[string]string)
	repos := make(map[string]string)
	nfs := make(map[string]string)
	manualIbackup := make(map[string][]dirSet)

	for _, root := range reportingRoots {
		ds, err := s.getRootSummary(root)
		if ds == nil || err != nil {
			continue
		}

		nds := &ruletree.DirSummary{
			RuleSummaries: ds.RuleSummaries,
			Children:      map[string]*ruletree.DirSummary{},
		}

		uid, gid := ds.IDs()
		nds.User = users.Username(uid)
		nds.Group = users.Group(gid)
		s.collectChildDirSummaries(nds, root)
		nds.ClaimedBy = s.getClaimed(root)
		dirSummary.Summaries[root] = nds

		s.collectRuleMetadata(ds, dirSummary, dirClaims, repos, nfs, manualIbackup)
	}

	s.populateBackupStatus(dirClaims, repos, nfs, manualIbackup, dirSummary)
}

func (s *Server) getRootSummary(root string) (*ruletree.DirSummary, error) {
	dr, ok := s.directoryRules[root]
	if !ok { //nolint:nestif
		ds, err := s.rootDir.Summary(root)
		if errors.Is(err, ruletree.ErrNotFound) || errors.As(err, new(tree.ChildNotFoundError)) {
			return nil, nil //nolint:nilnil
		} else if err != nil {
			return nil, err
		}

		return ds, nil
	} else if dr.DirSummary == nil {
		return nil, nil //nolint:nilnil
	}

	return dr.DirSummary, nil
}

func (s *Server) collectChildDirSummaries(ds *ruletree.DirSummary, root string) {
	for _, dir := range s.dirs {
		if strings.HasPrefix(dir.Path, root) && dir.Path != root {
			dir, exists := s.directoryRules[dir.Path]
			if !exists || dir == nil || dir.DirSummary == nil {
				continue
			}

			child := dir.DirSummary

			nchild := *dir.DirSummary
			nchild.Children = map[string]*ruletree.DirSummary{}

			uid, gid := child.IDs()

			nchild.User = users.Username(uid)
			nchild.Group = users.Group(gid)

			nchild.ClaimedBy = s.getClaimed(dir.Path)
			ds.Children[dir.Path] = &nchild
		}
	}
}

func (s *Server) collectRuleMetadata(ds *ruletree.DirSummary, dirSummary *summary, //nolint:gocyclo
	dirClaims, repos, nfs map[string]string, manualIbackup map[string][]dirSet,
) {
	for _, ruleSummary := range ds.RuleSummaries {
		rule := s.rules[ruleSummary.ID]

		dirID := rule.DirID()
		if dirID <= 0 {
			continue
		}

		dirPath := s.dirs[uint64(dirID)].Path
		dir := s.directoryRules[dirPath]

		switch rule.BackupType { //nolint:exhaustive
		case db.BackupIBackup:
			dirClaims[dir.Path] = dir.ClaimedBy
		case db.BackupManualIBackup:
			manualIbackup[dir.ClaimedBy] = append(manualIbackup[dir.ClaimedBy], dirSet{dir.Path, rule.Metadata})
		case db.BackupManualGit:
			repos[rule.Metadata] = dir.ClaimedBy
		case db.BackupManualNFS:
			nfs[rule.Metadata] = dir.ClaimedBy
		}

		if _, ok := dirSummary.Directories[dirPath]; ok {
			continue
		}

		s.collectRules(dirSummary, dir.DirRules)
	}
}

func (s *Server) collectRules(dirSummary *summary, dir *ruletree.DirRules) {
	ruleIDs := make([]uint64, 0, len(dir.Rules))

	for _, r := range dir.Rules {
		id := r.ID()
		if id < 0 {
			continue
		}

		ruleIDs = append(ruleIDs, uint64(id))
		dirSummary.Rules[uint64(id)] = s.rules[uint64(id)]
	}

	slices.Sort(ruleIDs)
	dirSummary.Directories[dir.Path] = ruleIDs
}
