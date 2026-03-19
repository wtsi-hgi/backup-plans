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
func Backup(planDB *db.DB, treeNode tree.Node, client *ibackup.MultiClient) ([]SetInfo, error) { //nolint:funlen
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

	setFofns := make(map[*db.Directory][]string)

	figureOutFOFNs(treeNode, sm, nil, func(path *summary.DirectoryPath, ruleID int64) {
		rule := dirRules[ruleID]

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

type backupClient interface {
	Backup(path string, setName, requester string, files []string,
		frequency int, frozen bool, review, remove int64) error
	GetBackupActivity(path, setName, requester string, manual bool) (*ibackup.SetBackupActivity, error)
}

func addFofnsToIBackup(client backupClient, setFofns map[*db.Directory][]string) ([]SetInfo, error) {
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
