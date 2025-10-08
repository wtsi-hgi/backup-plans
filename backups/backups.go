package backups

import (
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

func createRuleGroups(planDB *db.DB) []ruleGroup {
	rules := planDB.ReadRules()
	dirs := make(map[int64]string)

	for dir := range planDB.ReadDirectories().Iter {
		dirs[dir.ID()] = dir.Path
	}

	var groups []ruleGroup

	rules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
		newgroup := ruleGroup{
			Path:  []byte(dirs[rule.DirID()] + rule.Match),
			Group: rule,
		}

		groups = append(groups, newgroup)

		return nil
	})

	return groups
}

func Backup(planDB *db.DB, treeNode tree.Node, client *server.Client) ([]string, error) {
	groups := createRuleGroups(planDB)
	sm, _ := group.NewStatemachine(groups)

	m := make(map[int64][]string)

	filePaths(treeNode, func(fi *summary.FileInfo) {
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

	dirs := make(map[int64][]string)
	for dir := range planDB.ReadDirectories().Iter {
		dirs[dir.ID()] = []string{dir.Path, dir.ClaimedBy}
	}

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

// filePaths calls the given cb with every absolute file path nested under the
// given root node. Directory paths are not returned.
func filePaths(root tree.Node, cb func(path *summary.FileInfo)) {
	callCBOnAllAbsoluteFilePaths(root, nil, 0, cb)
}

func callCBOnAllAbsoluteFilePaths(node tree.Node, parent *summary.DirectoryPath, depth int, cb func(path *summary.FileInfo)) {
	for name, childnode := range node.Children() {
		if strings.HasSuffix(name, "/") {
			current := &summary.DirectoryPath{
				Name:   name,
				Depth:  depth,
				Parent: parent,
			}
			callCBOnAllAbsoluteFilePaths(childnode, current, depth+1, cb)
		} else {
			fi := &summary.FileInfo{
				Path: parent,
				Name: []byte(name),
			}
			cb(fi)
		}
	}
}

// // code to make a ruleList from info in db in a function in this pkg:
// ruleList := []group.PathGroup[db.Rule] {
// 	{
// 		Path: []byte(dirA.Path),
// 		Group: ruleA,
// 	},
// 	{
// 		Path: []byte(dirA.Path),
// 		Group: ruleB,
// 	},
// 	{
// 		Path: []byte(dirB.Path),
// 		Group: ruleC,
// 	},
// }

// sm, err := group.NewStatemachine(ruleList)
// So(err, ShouldBeNil)
