package backups

import (
	"bytes"
	"os/user"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	_ "modernc.org/sqlite"
	"vimagination.zapto.org/tree"
)

func TestFilePaths(t *testing.T) {
	Convey("Given a tree of wrstat info you can get all the absolute file paths", t, func() {
		tr := exampleTree()

		var paths []*summary.FileInfo

		filePaths(tr, func(path *summary.FileInfo) {
			paths = append(paths, path)
		})

		tests := []struct {
			index   int
			name    string
			depth   int
			dir     string
			parents []string
		}{
			{0, "1.jpg", 2, "b/", []string{"a/", "/"}},
			{1, "2.jpg", 2, "b/", []string{"a/", "/"}},
			{2, "3.txt", 2, "b/", []string{"a/", "/"}},
			{3, "temp.jpg", 2, "b/", []string{"a/", "/"}},
			{4, "4.txt", 2, "c/", []string{"a/", "/"}},
		}

		for _, test := range tests {
			path := paths[test.index]
			dir := path.Path

			So(string(path.Name), ShouldEqual, test.name)
			So(dir.Name, ShouldEqual, test.dir)
			So(dir.Depth, ShouldEqual, test.depth)

			parent := dir.Parent
			for _, actual := range test.parents {
				So(parent.Name, ShouldEqual, actual)
				parent = parent.Parent
			}
		}

	})
}

func exampleTree() tree.Node {
	dirRoot := directories.NewRoot("/", 12345)

	dirRoot.AddDirectory("a").SetMeta(99, 98, 1).AddDirectory("b").SetMeta(1, 1, 98765)
	directories.AddFile(&dirRoot.Directory, "a/b/1.jpg", 1, 1, 9, 98766)
	directories.AddFile(&dirRoot.Directory, "a/b/2.jpg", 1, 2, 8, 98767)
	directories.AddFile(&dirRoot.Directory, "a/b/3.txt", 1, 2, 8, 98767)
	directories.AddFile(&dirRoot.Directory, "a/b/temp.jpg", 1, 2, 8, 98767)

	dirRoot.AddDirectory("a").AddDirectory("c").SetMeta(2, 1, 12349)
	directories.AddFile(&dirRoot.Directory, "a/c/4.txt", 2, 1, 6, 12346)

	var treeDB bytes.Buffer

	So(tree.Serialise(&treeDB, dirRoot), ShouldBeNil)

	tr, err := tree.OpenMem(treeDB.Bytes())
	So(err, ShouldBeNil)

	return tr
}

func TestRuleToGroups(t *testing.T) {
	Convey("Given a plan database, you can create a slice of ruleGroups", t, func() {
		testDB := examplePlanDB(t)

		rgs := createRuleGroups(testDB)
		So(len(rgs), ShouldEqual, 3)

		var rules []*db.Rule

		readRules := testDB.ReadRules()

		readRules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
			rules = append(rules, rule)

			return nil
		})

		So(rgs, ShouldResemble, []ruleGroup{
			{
				Path:  []byte("/a/b/" + rules[0].Match),
				Group: rules[0],
			},
			{
				Path:  []byte("/a/b/" + rules[1].Match),
				Group: rules[1],
			},
			{
				Path:  []byte("/a/c/" + rules[2].Match),
				Group: rules[2],
			},
		})

	})
}

func examplePlanDB(t *testing.T) *db.DB {
	testDB := testdb.CreateTestDatabase(t)

	userA := "userA"
	userB := "userB"

	dirA := &db.Directory{
		Path:      "/a/b/",
		ClaimedBy: userA,
	}
	dirB := &db.Directory{
		Path:      "/a/c/",
		ClaimedBy: userB,
	}

	So(testDB.CreateDirectory(dirA), ShouldBeNil)
	So(testDB.CreateDirectory(dirB), ShouldBeNil)

	ruleA := &db.Rule{
		BackupType: db.BackupIBackup,
		Match:      "*.jpg",
		Frequency:  7,
	}
	ruleB := &db.Rule{
		BackupType: db.BackupNone,
		Match:      "temp.jpg",
		Frequency:  7,
	}
	ruleC := &db.Rule{
		BackupType: db.BackupManual,
		Match:      "*.txt",
		Metadata:   "manualSetName",
	}

	So(testDB.CreateDirectoryRule(dirA, ruleA), ShouldBeNil)
	So(testDB.CreateDirectoryRule(dirA, ruleB), ShouldBeNil)
	So(testDB.CreateDirectoryRule(dirB, ruleC), ShouldBeNil)

	return testDB
}

func TestBackups(t *testing.T) {
	Convey("Given a plan database, a tree of wrstat info and an ibackup server", t, func() {
		s, addr, certPath, dfn, err := internal.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		u, err := user.Current()
		So(err, ShouldBeNil)

		ibackupClient, err := ibackup.Connect(addr, certPath)
		So(err, ShouldBeNil)

		So(s, ShouldNotBeNil)
		So(u, ShouldNotBeNil)
		So(ibackupClient, ShouldNotBeNil)

		testDB := examplePlanDB(t)
		tr := exampleTree()

		Convey("You can create ibackup sets for all automatic ibackup plans, excluding BackupNone and BackupManual", func() {
			setIDs, err := Backup(testDB, tr, ibackupClient)
			So(err, ShouldBeNil)
			So(len(setIDs), ShouldEqual, 1)

			sets, err := ibackupClient.GetSets("userA")
			So(err, ShouldBeNil)
			So(len(sets), ShouldEqual, 1)
			So(sets[0].ID, ShouldEqual, setIDs[0])
			So(sets[0].Name, ShouldEqual, "plan::/a/b")
			So(sets[0].Requester, ShouldEqual, "userA")
		})
	})
}
