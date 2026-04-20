/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *         Sky Haines <sh55@sanger.ac.uk>
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

package backups

import (
	"errors"
	"strings"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/tree"
)

const (
	setNamePrefix = "plan::"
)

type SetInfo struct {
	BackupSetName string
	Requestor     string
	FileCount     int
}

// Backup will back up all files in the given treeNode that match rules in the
// given planDB, using the given ibackup client. It returns a list of the set IDs
// created.
func Backup(planDB *db.DB, treeNode *tree.MemTree, client *ibackup.MultiClient) ([]SetInfo, error) { //nolint:funlen
	mountpoint, err := readMountpoint(treeNode)
	if err != nil {
		return nil, err
	}

	dirs, dirRules, err := readDirRules(planDB, mountpoint)
	if err != nil {
		return nil, err
	}

	root := ruletree.NewRuleTree()

	for _, dr := range dirs {
		root.Set(dr.Path, dr.Rules, false)
	}

	root.Canon()
	root.MarkBackupDirs()

	rules := root.BuildRules()

	rules[0] = collectRuleGroups(root, "/", rules[0])

	sm, err := ruletree.BuildMultiStateMachine(rules)
	if err != nil {
		return nil, err
	}

	setFofns := make(map[*db.Directory][]server.PathMTime)

	figureOutFOFNs(treeNode, sm, nil, func(path *summary.DirectoryPath, mtime, ruleID int64) {
		rule := dirRules[ruleID]

		if rule.RuleIDs[ruleID].BackupType == db.BackupIBackup {
			setFofns[rule.Directory] = append(setFofns[rule.Directory], server.PathMTime{
				Path:  string(path.AppendTo(nil)),
				MTime: mtime,
			})
		}
	})

	return addFofnsToIBackup(client, setFofns)
}

func readMountpoint(treeNode *tree.MemTree) (string, error) {
	var (
		mountpoint string
		err        error
	)

	treeNode.Children()(func(name string, n tree.Node) bool {
		if cerr, ok := n.(tree.ChildrenError); ok {
			err = cerr.Unwrap()
		}

		mountpoint = name

		return false
	})

	return mountpoint, err
}

type dirRules struct {
	*db.Directory
	Rules   map[string]*db.Rule
	RuleIDs map[int64]*db.Rule
}

func readDirRules(planDB *db.DB, mountpoint string) (map[int64]*dirRules, map[int64]*dirRules, error) {
	dirs := make(map[int64]*dirRules)
	rules := make(map[int64]*dirRules)

	if err := planDB.ReadDirectories().ForEach(func(dir *db.Directory) error {
		if strings.HasPrefix(dir.Path, mountpoint) {
			dirs[dir.ID()] = &dirRules{
				Directory: dir,
				Rules:     make(map[string]*db.Rule),
				RuleIDs:   make(map[int64]*db.Rule),
			}
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	if err := planDB.ReadRules().ForEach(func(rule *db.Rule) error {
		if dir, ok := dirs[rule.DirID()]; ok {
			dir.Rules[rule.Match] = rule
			dir.RuleIDs[rule.ID()] = rule
			rules[rule.ID()] = dir
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	return dirs, rules, nil
}

var hasBackups int64 = -1 //nolint:gochecknoglobals

func collectRuleGroups(root *ruletree.RuleTree, path string, rules ruletree.Rules) ruletree.Rules {
	if !root.HasBackup && !root.HasChildWithBackup {
		return rules
	}

	rules = append(rules, group.PathGroup[int64]{
		Path:  []byte(path),
		Group: &hasBackups,
	})

	if root.HasBackup {
		rules = append(rules, group.PathGroup[int64]{
			Path:  []byte(path + "*/"),
			Group: &hasBackups,
		})
	}

	for name, rt := range root.Iter() {
		rules = collectRuleGroups(rt, path+name, rules)
	}

	return rules
}

func figureOutFOFNs(node tree.Node, sm ruletree.State, path *summary.DirectoryPath,
	cb func(*summary.DirectoryPath, int64, int64)) {
	for name, child := range node.Children() {
		state := sm.GetStateString(name)
		newPath := &summary.DirectoryPath{Parent: path, Name: name}
		group := state.GetGroup()

		if group == nil {
			continue
		}

		if !strings.HasSuffix(name, "/") {
			cb(newPath, readMTime(child), *group)

			continue
		}

		if *group != hasBackups {
			continue
		}

		figureOutFOFNs(child, state, newPath, cb)
	}
}

func readMTime(child tree.Node) int64 {
	return int64(ruletree.ReadFileStats(child.(*tree.MemTree)).MTime)
}

type backupClient interface {
	Backup(path string, setName, requester string, files []server.PathMTime,
		frequency int, frozen bool, review, remove int64) error
	GetBackupActivity(path, setName, requester string, manual bool) (*ibackup.SetBackupActivity, error)
}

func addFofnsToIBackup(client backupClient, setFofns map[*db.Directory][]server.PathMTime) ([]SetInfo, error) {
	backupSetInfos := make([]SetInfo, 0, len(setFofns))

	var errs error

	for setInfo, fofns := range setFofns {
		backupSetName := setNamePrefix + setInfo.Path

		frozen, err := getFrozenStatus(client, setInfo, backupSetName)
		if err != nil {
			errs = errors.Join(errs, err)

			continue
		}

		err = client.Backup(setInfo.Path, backupSetName, setInfo.ClaimedBy, fofns,
			int(setInfo.Frequency), frozen, setInfo.ReviewDate, setInfo.RemoveDate) //nolint:gosec
		if err != nil {
			errs = errors.Join(errs, err)

			continue
		}

		backupSetInfos = append(backupSetInfos, SetInfo{
			BackupSetName: backupSetName,
			Requestor:     setInfo.ClaimedBy,
			FileCount:     len(fofns),
		})
	}

	return backupSetInfos, errs
}

func getFrozenStatus(client backupClient, setInfo *db.Directory, backupSetName string) (bool, error) {
	frozen := setInfo.Frozen

	if !frozen || setInfo.Melt == 0 {
		return frozen, nil
	}

	set, err := client.GetBackupActivity(setInfo.Path, backupSetName, setInfo.ClaimedBy, false)
	if err != nil {
		if errors.Is(err, server.ErrBadSet) {
			return true, nil
		}

		return true, err
	}

	return time.Unix(setInfo.Melt, 0).After(set.LastSuccess), nil
}
