package backups

import (
	"errors"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/ruletree"
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
func Backup(planDB *db.DB, treeNode tree.Node, client *ibackup.MultiClient) ([]SetInfo, error) { //nolint:funlen
	mountpoint, err := readMountpoint(treeNode)
	if err != nil {
		return nil, err
	}

	dirs, rules, err := readDirRules(planDB, mountpoint)
	if err != nil {
		return nil, err
	}

	root := ruletree.NewRuleTree()

	for _, dr := range dirs {
		root.Set(dr.Path, dr.Rules, false)
	}

	root.Canon()
	root.MarkBackupDirs()

	groups := collectRuleGroups(root, "/", nil)

	sm, err := group.NewStatemachine(groups)
	if err != nil {
		return nil, err
	}

	setFofns := make(map[*db.Directory][]string)

	figureOutFOFNs(treeNode, sm.GetStateString(""), nil, func(path *summary.DirectoryPath, ruleID int64) {
		rule := rules[ruleID]

		if rule.RuleIDs[ruleID].BackupType == db.BackupIBackup {
			setFofns[rule.Directory] = append(setFofns[rule.Directory], string(path.AppendTo(nil)))
		}
	})

	return addFofnsToIBackup(client, setFofns)
}

func readMountpoint(treeNode tree.Node) (string, error) {
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

func collectRuleGroups(root *ruletree.RuleTree, path string, rules []group.PathGroup[int64]) []group.PathGroup[int64] {
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

	for _, rule := range root.Rules {
		id := rule.ID()

		rules = append(rules, group.PathGroup[int64]{
			Path:  []byte(path + rule.Match),
			Group: &id,
		})
	}

	for name, rt := range root.Iter() {
		rules = collectRuleGroups(rt, path+name, rules)
	}

	return rules
}

func figureOutFOFNs(node tree.Node, sm group.State[int64], path *summary.DirectoryPath,
	cb func(*summary.DirectoryPath, int64)) {
	for name, child := range node.Children() {
		state := sm.GetStateString(name)
		newPath := &summary.DirectoryPath{Parent: path, Name: name}
		group := state.GetGroup()

		if group == nil {
			continue
		}

		if !strings.HasSuffix(name, "/") {
			cb(newPath, *group)

			continue
		}

		if *group != hasBackups {
			continue
		}

		figureOutFOFNs(child, state, newPath, cb)
	}
}

func addFofnsToIBackup(client *ibackup.MultiClient, setFofns map[*db.Directory][]string) ([]SetInfo, error) {
	backupSetInfos := make([]SetInfo, 0, len(setFofns))

	var errs error

	for setInfo, fofns := range setFofns {
		backupSetName := setNamePrefix + setInfo.Path

		err := client.Backup(setInfo.Path, backupSetName, setInfo.ClaimedBy, fofns,
			int(setInfo.Frequency), setInfo.ReviewDate, setInfo.RemoveDate) //nolint:gosec
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

	return backupSetInfos, nil
}
