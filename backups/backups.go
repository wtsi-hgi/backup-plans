package backups

import (
	"fmt"
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
	setNamePrefix = "plan::"
)

func createRuleGroups(planDB *db.DB, dirs map[int64][]string) ([]ruleGroup, map[string]bool) {
	rules := planDB.ReadRules()

	var groups []ruleGroup
	dirsWithRules := make(map[string]bool)

	rules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
		path := dirs[rule.DirID()][0]
		newgroup := ruleGroup{
			Path:  []byte(path + rule.Match),
			Group: rule,
		}

		groups = append(groups, newgroup)

		// Add path and all parent paths to dirsWithRules
		pathToAdd := strings.TrimRight(path, "/")
		for {
			if pathToAdd == "/" {
				dirsWithRules[pathToAdd] = true
				break
			}

			if dirsWithRules[pathToAdd+"/"] {
				fmt.Printf("\n Already have %s in %+v.", pathToAdd+"/", dirsWithRules)
				break
			}

			fmt.Printf("\n Adding %s", pathToAdd+"/")
			dirsWithRules[pathToAdd+"/"] = true
			pathToAdd = filepath.Dir(pathToAdd)
		}

		return nil
	})

	return groups, dirsWithRules
}

func Backup(planDB *db.DB, treeNode tree.Node, client *server.Client) ([]string, error) {
	dirs := make(map[int64][]string)

	for dir := range planDB.ReadDirectories().Iter {
		dirs[dir.ID()] = []string{dir.Path, dir.ClaimedBy}
	}

	groups, dirsWithRules := createRuleGroups(planDB, dirs)
	sm, _ := group.NewStatemachine(groups)

	m := make(map[int64][]string)

	fileInfos(treeNode, dirsWithRules, func(fi *summary.FileInfo) {
		rule := sm.GetGroup(fi)
		if rule == nil {
			return
		}

		if rule.BackupType == db.BackupManual || rule.BackupType == db.BackupNone {
			return
		}

		m[rule.DirID()] = append(m[rule.DirID()], string(fi.Path.AppendTo(nil))+string(fi.Name))
	})

	var setIDs []string

	for dirId, fofns := range m {
		setInfo := dirs[dirId]
		setID, err := ibackup.Backup(client, setNamePrefix+setInfo[0], setInfo[1], fofns, 7)
		if err != nil {
			return nil, err
		}

		setIDs = append(setIDs, setID)
	}

	return setIDs, nil
}

// fileInfos calls the given cb with every absolute file path nested under the
// given root node. Directory paths are not returned.
//
// TODO: have this skip directories which we know don't have rules; take a map
// of directory paths that have rules (ie. taken from plan db ReadRules) and
// that info to callCBOnAllAbsoluteFilePaths so it can skip (not recurse) dirs
// with no rules.
func fileInfos(root tree.Node, dirsWithRules map[string]bool, cb func(path *summary.FileInfo)) {
	callCBOnAllFiles(root, dirsWithRules, nil, 0, cb)
}

func callCBOnAllFiles(node tree.Node, dirsWithRules map[string]bool, parent *summary.DirectoryPath, depth int, cb func(path *summary.FileInfo)) {
	for name, childnode := range node.Children() {
		if strings.HasSuffix(name, "/") {
			current := &summary.DirectoryPath{
				Name:   name,
				Depth:  depth,
				Parent: parent,
			}

			dirPath := string(current.AppendTo(nil))
			if dirsWithRules[dirPath] {
				callCBOnAllFiles(childnode, dirsWithRules, current, depth+1, cb)
			} else {
				fmt.Printf("\n Dirpath %s not in dirsWithRules, skipping.", dirPath)
			}

		} else {
			fi := &summary.FileInfo{
				Path: parent,
				Name: []byte(name),
			}

			cb(fi)
		}
	}
}
