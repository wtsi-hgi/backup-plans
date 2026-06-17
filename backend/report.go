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
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/backup-plans/ruletree"
)

type summary struct {
	Summaries             map[string]*ruletree.DirSummary
	Rules                 map[uint64]rules.Rule
	Directories           map[string][]uint64
	BackupStatus          map[string]ibackup.SetBackupActivity
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
		Rules:                 make(map[uint64]rules.Rule),
		Directories:           make(map[string][]uint64),
		BackupStatus:          make(map[string]ibackup.SetBackupActivity),
		GroupBackupTypeTotals: make(map[string]map[int]*SizeCount),
	}

	err := s.collectBackupTotals(&dirSummary)
	if err != nil {
		return err
	}

	s.buildRootDirSummary(reportingRoots, &dirSummary)

	w.Header().Set("Content-type", "application/json")

	return json.NewEncoder(w).Encode(dirSummary)
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

func (s *Server) getIBackupBackupStatus(planName, dir, claimedBy string) ibackup.SetBackupActivity {
	sbaPtr, err := s.config.GetCachedIBackupClient().GetBackupActivity(dir, planName, claimedBy, false)
	if err != nil {
		slog.Error("error querying ibackup status", "dir", dir, "err", err)
	}

	if sbaPtr != nil {
		return *sbaPtr
	}

	return ibackup.SetBackupActivity{
		Name:      planName,
		Requester: claimedBy,
	}
}

func (s *Server) populateManualIBackupStatus(manualIbackup map[string][]dirSet, dirSummary *summary) {
	for claimedBy, dirSets := range manualIbackup {
		for _, dirSet := range dirSets {
			sba := s.getManualIBackupStatus(dirSet, claimedBy)
			dirSummary.BackupStatus[claimedBy+":"+dirSet.set] = sba
		}
	}
}

func (s *Server) getManualIBackupStatus(dirSet dirSet, claimedBy string) ibackup.SetBackupActivity {
	sbaPtr, err := s.config.GetCachedIBackupClient().GetBackupActivity(dirSet.dir, dirSet.set, claimedBy, true)
	if err != nil {
		slog.Error("error querying manual ibackup status",
			"dir", dirSet.dir, "claimedBy", claimedBy, "set", dirSet.set, "err", err)
	}

	if sbaPtr != nil {
		return *sbaPtr
	}

	return ibackup.SetBackupActivity{
		Name:      dirSet.set,
		Requester: claimedBy,
	}
}

func (s *Server) populateGitBackupStatus(repos map[string]string, dirSummary *summary) {
	for repo, claimedBy := range repos {
		dirSummary.BackupStatus[repo] = s.getGitBackupStatus(repo, claimedBy)
	}
}

func (s *Server) getGitBackupStatus(repo, claimedBy string) ibackup.SetBackupActivity {
	t, err := s.gitCache.GetLatestCommitDate(repo)
	if err != nil {
		slog.Error("error querying repo status", "repo", repo, "err", err)
	}

	sba := ibackup.SetBackupActivity{
		LastSuccess: t,
		Name:        repo,
		Requester:   claimedBy,
		Failures:    -1,
	}

	return sba
}

func (s *Server) populateNFSStatus(backupPaths map[string]string, dirSummary *summary) {
	for backupPath, claimedBy := range backupPaths {
		sba := s.getNFSStatus(backupPath, claimedBy)

		dirSummary.BackupStatus["nfs:"+backupPath] = sba
	}
}

func (s *Server) getNFSStatus(backupPath, claimedBy string) ibackup.SetBackupActivity {
	sba := ibackup.SetBackupActivity{
		Name:      backupPath,
		Requester: claimedBy,
		Failures:  -1,
	}

	client := s.config.GetWRStatClient()

	t, err := client.GetWRStatModTime(backupPath)
	if err != nil {
		slog.Error("error querying wrstat status", "path", backupPath, "err", err)
	}

	sba.LastSuccess = t

	return sba
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

	rule := s.rootDir.Rule(id)
	if rule == nil {
		return unplanned
	}

	return int(rule.BackupType)
}

func (s *Server) buildRootDirSummary(reportingRoots []string, dirSummary *summary) {
	dirClaims := make(map[string]string)
	repos := make(map[string]string)
	nfs := make(map[string]string)
	manualIbackup := make(map[string][]dirSet)

	for _, root := range reportingRoots {
		ds, err := s.rootDir.Summary(root)
		if ds == nil || err != nil {
			continue
		}

		nds := &ruletree.DirSummary{
			RuleSummaries: ds.RuleSummaries,
			Children:      map[string]*ruletree.DirSummary{},
			LastMod:       ds.LastMod,
			User:          ds.User,
			Group:         ds.Group,
			ClaimedBy:     ds.ClaimedBy,
		}

		s.collectChildDirSummaries(nds, root)
		dirSummary.Summaries[root] = nds

		s.collectRuleMetadata(ds, dirSummary, dirClaims, repos, nfs, manualIbackup)
	}

	s.populateBackupStatus(dirClaims, repos, nfs, manualIbackup, dirSummary)
}

func (s *Server) collectChildDirSummaries(ds *ruletree.DirSummary, root string) {
	for dir, summary := range s.rootDir.ClaimedSummaries() {
		if strings.HasPrefix(dir, root) && dir != root && summary != nil {
			nchild := *summary
			nchild.Children = map[string]*ruletree.DirSummary{}
			ds.Children[dir] = &nchild
		}
	}
}

func (s *Server) collectRuleMetadata(ds *ruletree.DirSummary, dirSummary *summary, //nolint:gocyclo,gocognit,funlen
	dirClaims, repos, nfs map[string]string, manualIbackup map[string][]dirSet,
) {
	for _, ruleSummary := range ds.RuleSummaries {
		if ruleSummary.ID <= 0 {
			continue
		}

		rule := s.rootDir.Rule(ruleSummary.ID)

		if rule == nil {
			continue
		}

		dir := s.rootDir.RuleDir(uint64(rule.ID)) //nolint:gosec
		if dir == nil {
			continue
		}

		switch rule.BackupType {
		case db.BackupIBackup:
			dirClaims[dir.Path] = dir.ClaimedBy
		case db.BackupManualIBackup:
			manualIbackup[dir.ClaimedBy] = append(manualIbackup[dir.ClaimedBy], dirSet{dir.Path, rule.Metadata})
		case db.BackupManualGit:
			repos[rule.Metadata] = dir.ClaimedBy
		case db.BackupManualNFS:
			nfs[rule.Metadata] = dir.ClaimedBy
		}

		if _, ok := dirSummary.Directories[dir.Path]; ok {
			continue
		}

		s.collectRules(dirSummary, dir.Path)
	}
}

func (s *Server) collectRules(dirSummary *summary, dir string) {
	ruleIDs := make([]uint64, 0) //nolint:prealloc

	for r := range s.rootDir.DirRules(dir) {
		ruleIDs = append(ruleIDs, uint64(r.ID)) //nolint:gosec
		dirSummary.Rules[uint64(r.ID)] = r      //nolint:gosec
	}

	slices.Sort(ruleIDs)
	dirSummary.Directories[dir] = ruleIDs
}

// FileList generates a CSV of files currently in the backup. By default it
// returns all backed up files under the specified directory that don't match
// current rules.
//
// If the 'matching' argument is not empty, it will returns files that match
// current rules.
//
// If the `single` argument is not empty, it will only return files that were
// backed up for the specified directory.
func (s *Server) FileList(w http.ResponseWriter, r *http.Request) {
	handle(w, r, s.fileList)
}

func (s *Server) fileList(w http.ResponseWriter, r *http.Request) error {
	dir, err := getDir(r)
	if err != nil {
		return err
	}

	recursive := r.FormValue("single") == ""
	matching := r.FormValue("matching") != ""
	csv := csv.NewWriter(w)

	defer csv.Flush()

	w.Header().Set("Content-type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filepath.Base(dir)+".csv"))

	return s.writeCSV(csv, dir, matching, recursive)
}

func (s *Server) writeCSV(csv *csv.Writer, dir string, matching, recursive bool) error { //nolint:gocognit,funlen
	ruleCache := make(map[uint64]bool)
	row := [3]string{"Remote Path", "Exists Locally", "Local Path"}

	if err := csv.Write(row[:]); err != nil {
		return err
	}

	return s.rootDir.BackedUpFiles(dir, recursive).ForEach(func(path string, stats ruletree.BackupStats) error {
		isBackup, ok := ruleCache[stats.RuleID]
		if !ok {
			if r := s.rootDir.Rule(stats.RuleID); r != nil {
				isBackup = r.BackupType == db.BackupIBackup
			}

			ruleCache[stats.RuleID] = isBackup
		}

		if isBackup != matching {
			return nil
		}

		row[2] = path
		row[0] = stats.RemotePath

		if stats.HasLocal {
			row[1] = "True"
		} else {
			row[1] = "False"
		}

		return csv.Write(row[:])
	})
}
