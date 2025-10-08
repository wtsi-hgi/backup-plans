package backups

import (
	"fmt"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/tree"
)

//TODO: the function we implement here calls db.ReadRules() to get
//rules from the db, and combines that with the paths from wrstat to
//get matching paths from statemachine.
//
// stores map of dirID to slice of file path strings, so that we can
// call ibackup.Backup() for the sets.

type ruleGroup = group.PathGroup[db.Rule]

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

	filePaths(treeNode, func(fi *summary.FileInfo) {
		rule := sm.GetGroup(fi)
		if rule == nil {
			return
		}
		fmt.Printf("\n Fileinfo: %+v Rule: %+v BackupType: %+v", string(fi.Name), rule.Match, rule.BackupType)
		if rule.BackupType == db.BackupManual || rule.BackupType == db.BackupNone {
			return
		}

		// stores map of dirID to slice of file path strings, so that we can
		// call ibackup.Backup() for the sets.
	})

	// since ret is nil, does this mean there are no matches in the gitignore so we need to backup the files?
	// if backup, get directory; add path to directory FOFN.

	// for _, group := range groups {
	// 	fmt.Printf("\n MATCH: %s", group.Group.Match)
	// }

	// createRuleGroups():
	// build map (map[uint64]*db.Directory) of directories
	// loop through rules, create slice of []ruleGroup
	// where Path is Directory.Path + rule.Match and Group is Rule
	// build statemachine from that slice

	// filePaths():
	// Walk treeNode, build file abs paths

	// run through statemachine to get rule

	return nil, nil
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

// // do something likd ruletree createRulePrefixMap to find dirs with
// // rules in them, then:
// // recursive function to talk the tree, along these lines:
// for name, child := range tr.Children() {
// 	if strings.HasSuffix(name, "/") {
// 		// dir, recurse if has rules
// 		child.Children()
// 	} else {
// 		// file, run it through rulemachine
// 	}
// }
