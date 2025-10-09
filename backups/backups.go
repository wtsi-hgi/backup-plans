package backups

import (
	"fmt"
	"path/filepath"
	"slices"
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

func createRuleGroups(planDB *db.DB, dirs map[int64][]string) ([]ruleGroup, map[string]bool, []string) {
	rules := planDB.ReadRules()

	var groups []ruleGroup
	var ruleList []string
	dirsWithRules := make(map[string]bool)

	rules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
		path := dirs[rule.DirID()][0]
		newgroup := ruleGroup{
			Path:  []byte(path + rule.Match),
			Group: rule,
		}

		ruleList = append(ruleList, string(path))
		groups = append(groups, newgroup)

		// Add path and all parent paths to dirsWithRules

		pathToAdd := strings.TrimRight(path, "/")
		for {
			if pathToAdd == "/" {
				dirsWithRules[pathToAdd] = true
				break
			}

			if dirsWithRules[pathToAdd+"/"] {
				// fmt.Printf("\n Already have %s in %+v.", pathToAdd+"/", dirsWithRules)
				break
			}

			// fmt.Printf("\n Adding %s", pathToAdd+"/")
			dirsWithRules[pathToAdd+"/"] = true
			pathToAdd = filepath.Dir(pathToAdd)
		}

		return nil
	})

	return groups, dirsWithRules, ruleList
}

func Backup(planDB *db.DB, treeNode tree.Node, client *server.Client) ([]string, error) {
	dirs := make(map[int64][]string)

	for dir := range planDB.ReadDirectories().Iter {
		dirs[dir.ID()] = []string{dir.Path, dir.ClaimedBy}
	}

	groups, dirsWithRules, ruleList := createRuleGroups(planDB, dirs)
	sm, _ := group.NewStatemachine(groups)

	m := make(map[int64][]string)

	fileInfos(treeNode, dirsWithRules, ruleList, func(fi *summary.FileInfo) {
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
func fileInfos(root tree.Node, dirsWithRules map[string]bool, ruleList []string, cb func(path *summary.FileInfo)) {
	findRuleDir(root, dirsWithRules, ruleList, nil, 0, cb)
}

// findRuleDir will recursively traverse only the tree directories with rules
// in them (dirsWithRules). When a directory in the ruleList is found, it will
// call callCBOnAllSubdirs on that node.
func findRuleDir(node tree.Node, dirsWithRules map[string]bool, ruleList []string, parent *summary.DirectoryPath, depth int, cb func(path *summary.FileInfo)) {
	for name, childnode := range node.Children() {
		// if strings.HasSuffix(name, "/") {
		current := &summary.DirectoryPath{
			Name:   name,
			Depth:  depth,
			Parent: parent,
		}

		dirPath := string(current.AppendTo(nil))
		if slices.Contains(ruleList, dirPath) {
			fmt.Printf("\n Dirpath %s in ruleList.", dirPath)
			// Backup everything in this dir and all subdirs
			callCBOnAllSubdirs(childnode, current, cb)
			continue
		} else if dirsWithRules[dirPath] {
			findRuleDir(childnode, dirsWithRules, ruleList, current, depth+1, cb)
		} else {
			fmt.Printf("\n Dirpath %s not in dirsWithRules.", dirPath)
			// TODO:
			// up everything under it regardless? opgtimise to use a map for O(1) lookups?
			// or add a flag to dirsWithRules to indicate if it is a rule dir or just a parent of one?
		}
		// }
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

			fmt.Printf("\n Calling cb on %s in callCBOnAllSubdirs", string(fi.Name))
			cb(fi)
		}
	}
}
