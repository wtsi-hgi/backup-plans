package backups

import (
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/tree"
)

//TODO: the function we implement here calls db.ReadRules() to get
//rules from the db, and combines that with the paths from wrstat to
//get matching paths from statemachine.
//
// stores map of dirID to slice of file path strings, so that we can
// call ibackup.Backup() for the sets.

type ruleGroup group.PathGroup[db.Rule]

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
	//sm, _ := group.NewStatemachine[db.Rule]()
	//sm.GetGroup()
	// FileInfo struct {
	// Path         *DirectoryPath // build this up as we go
	// Name         []byte // basename

	// createRuleGroups():
	// build map (map[uint64]*db.Directory) of directories
	// loop through rules, create slice of []ruleGroup
	// where Path is Directory.Path + rule.Match and Group is Rule
	// build statemachine from that slice

	// filePaths():
	// Walk treeNode, build file abs paths

	// run through statemachine to get rule
	// if backup, get directory; add path to directory FOFN.

	return nil, nil
}

// filePaths calls the given cb with every absolute file path nested under the
// given root node. Directory paths are not returned.
func filePaths(root tree.Node, cb func(path string)) {
	callCBOnAllAbsoluteFilePaths(root, "", cb)
}

func callCBOnAllAbsoluteFilePaths(node tree.Node, currentpath string, cb func(string)) {
	for name, childnode := range node.Children() {
		nextpath := currentpath + name
		if strings.HasSuffix(name, "/") {
			callCBOnAllAbsoluteFilePaths(childnode, nextpath, cb)
		} else {
			cb(nextpath)
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
