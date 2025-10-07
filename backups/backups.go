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

func Backup(planDB *db.DB, treeNode tree.Node, client *server.Client) ([]string, error) {
	iterRules := planDB.ReadRules()

	iterRules.ForEach(func(r *db.Rule) error {
		// fmt.Printf("rule: %+v", r)
		return nil
	})

	//sm, _ := group.NewStatemachine[db.Rule]()
	//sm.GetGroup()
	// FileInfo struct {
	// Path         *DirectoryPath // build this up as we go
	// Name         []byte // basename

	type ruleGroup group.PathGroup[db.Rule]

	planDB.ReadDirectories()
	// build map (map[uint64]*db.Directory) of directories
	// loop through rules, create slice of []ruleGroup
	// where Path is Directory.Path + rule.Match and Group is Rule
	// buikd statwmachine from from that slice

	// Walk treeNode, build file abs paths, run through statemachine to get rule
	// if backup, get directory; add path to directory FOFN.

	return nil, nil
}

func filePaths(root tree.Node, cb func(path string)) {
	helper(root, "", cb)
}

func helper(node tree.Node, currentpath string, cb func(string)) {
	for name, childnode := range node.Children() {
		nextpath := currentpath + name
		if strings.HasSuffix(name, "/") {
			helper(childnode, nextpath, cb)
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
