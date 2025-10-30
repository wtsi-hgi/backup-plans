package backups

import (
	"path/filepath"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/tree"
)

type ruleGroup = group.PathGroup[db.Rule]

const (
	setNamePrefix    = "plan::"
	defaultFrequency = 7
)

func createRuleGroups(planDB *db.DB, dirs map[int64]*db.Directory) ([]ruleGroup, map[string]struct{}) {
	rules := planDB.ReadRules()

	var groups []ruleGroup

	ruleList := make(map[string]struct{})

	rules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
		path := dirs[rule.DirID()].Path
		newgroup := ruleGroup{
			Path:  []byte(path + rule.Match),
			Group: rule,
		}

		ruleList[path] = struct{}{}

		groups = append(groups, newgroup)

		return nil
	})

	return groups, ruleList
}

type SetInfo struct {
	BackupSetName string
	Requestor     string
	FileCount     int
}

// Backup will back up all files in the given treeNode that match rules in the
// given planDB, using the given ibackup client. It returns a list of the set IDs
// created.
func Backup(planDB *db.DB, treeNode tree.Node, client *server.Client) ([]SetInfo, error) {
	dirs := make(map[int64]*db.Directory)

	for dir := range planDB.ReadDirectories().Iter {
		dirs[dir.ID()] = dir
	}

	groups, ruleList := createRuleGroups(planDB, dirs)
	sm, _ := group.NewStatemachine(groups) //nolint:errcheck

	setFofns := make(map[int64][]string)

	fileInfos(treeNode, ruleList, func(fi *summary.FileInfo) {
		rule := sm.GetGroup(fi)
		if rule == nil {
			return
		}

		if rule.BackupType == db.BackupManual || rule.BackupType == db.BackupNone {
			return
		}

		setFofns[rule.DirID()] = append(setFofns[rule.DirID()], string(fi.Path.AppendTo(nil))+string(fi.Name))
	})

	return addFofnsToIBackup(client, setFofns, dirs)
}

func addFofnsToIBackup(client *server.Client, setFofns map[int64][]string,
	dirs map[int64]*db.Directory) ([]SetInfo, error) {
	backupSetInfos := make([]SetInfo, 0, len(setFofns))

	for dirID, fofns := range setFofns {
		setInfo := dirs[dirID]
		backupSetName := setNamePrefix + setInfo.Path

		err := ibackup.Backup(client, backupSetName, setInfo.ClaimedBy, fofns, defaultFrequency)
		if err != nil {
			return nil, err
		}

		backupSetInfos = append(backupSetInfos, SetInfo{
			BackupSetName: backupSetName,
			Requestor:     setInfo.ClaimedBy,
			FileCount:     len(fofns),
		})
	}

	return backupSetInfos, nil
}

// fileInfos calls the given cb with every absolute file path nested under the
// given root node for which there is a matching rule.
// Directory paths are not returned.
func fileInfos(root tree.Node, ruleList map[string]struct{}, cb func(path *summary.FileInfo)) {
	dirsWithRules := make(map[string]bool)

	for rule := range ruleList {
		pathToAdd := strings.TrimRight(rule, "/")
		for {
			if pathToAdd == "/" {
				dirsWithRules[pathToAdd] = true

				break
			}

			if dirsWithRules[pathToAdd+"/"] {
				break
			}

			dirsWithRules[pathToAdd+"/"] = true
			pathToAdd = filepath.Dir(pathToAdd)
		}
	}

	findRuleDir(root, dirsWithRules, ruleList, nil, cb)
}

// findRuleDir will recursively traverse only the tree directories with rules
// in them (dirsWithRules). When a directory in the ruleList is found, it will
// call callCBOnAllSubdirs on that node.
func findRuleDir(node tree.Node, dirsWithRules map[string]bool,
	ruleList map[string]struct{}, parent *summary.DirectoryPath,
	cb func(path *summary.FileInfo)) {
	for name, childnode := range node.Children() {
		depth := 0
		if parent != nil {
			depth = parent.Depth + 1
		}

		current := &summary.DirectoryPath{
			Name:   name,
			Depth:  depth,
			Parent: parent,
		}

		dirPath := string(current.AppendTo(nil))
		if _, exists := ruleList[dirPath]; exists {
			callCBOnAllSubdirs(childnode, current, cb)

			continue
		} else if dirsWithRules[dirPath] {
			findRuleDir(childnode, dirsWithRules, ruleList, current, cb)
		}
	}
}

// callCBOnAllSubdirs will create a FileInfo for every file in every directory
// nested under the given node, and return it to cb.
func callCBOnAllSubdirs(node tree.Node, parent *summary.DirectoryPath, cb func(path *summary.FileInfo)) {
	for name, childnode := range node.Children() {
		if strings.HasSuffix(name, "/") {
			current := &summary.DirectoryPath{
				Name:   name,
				Depth:  parent.Depth + 1,
				Parent: parent,
			}

			callCBOnAllSubdirs(childnode, current, cb)
		} else {
			fi := &summary.FileInfo{
				Path: parent,
				Name: []byte(name),
			}

			cb(fi)
		}
	}
}
