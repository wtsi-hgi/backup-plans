package backups

import (
	"bytes"
	"os/user"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	internal "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	_ "modernc.org/sqlite"
	"vimagination.zapto.org/tree"
)

func TestFileInfos(t *testing.T) {
	Convey("Given a tree of wrstat info you can get all the file infos", t, func() {
		tr := exampleTree()

		var paths []*summary.FileInfo

		ruleList := map[string]struct{}{
			"/lustre/scratch123/humgen/a/b/": {},
		}

		fileInfos(tr, ruleList, func(path *summary.FileInfo) {
			paths = append(paths, path)
		})

		So(len(paths), ShouldEqual, 5)

		humgenDir := []string{"a/", "humgen/", "scratch123/", "lustre/", "/"}

		tests := []struct {
			index   int
			name    string
			depth   int
			dir     string
			parents []string
		}{
			{0, "1.jpg", 5, "b/", humgenDir},
			{1, "2.jpg", 5, "b/", humgenDir},
			{2, "3.txt", 5, "b/", humgenDir},
			{3, "temp.jpg", 5, "b/", humgenDir},
			{4, "test.txt", 6, "testdir/", append([]string{"b/"}, humgenDir...)},
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
	humgen := dirRoot.AddDirectory("lustre").SetMeta(99, 98, 1).AddDirectory("scratch123").
		SetMeta(1, 1, 98765).AddDirectory("humgen").SetMeta(1, 1, 98765)

	humgen.AddDirectory("a").SetMeta(99, 98, 1).AddDirectory("b").SetMeta(1, 1, 98765).
		AddDirectory("testdir").SetMeta(2, 1, 12349)
	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/a/b/1.jpg", 1, 1, 9, 98766)
	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/a/b/2.jpg", 1, 2, 8, 98767)
	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/a/b/3.txt", 1, 2, 8, 98767)
	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/a/b/temp.jpg", 1, 2, 8, 98767)
	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/a/b/testdir/test.txt", 2, 1, 6, 12346)

	humgen.AddDirectory("a").AddDirectory("c").SetMeta(2, 1, 12349)
	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/a/c/4.txt", 2, 1, 6, 12346)

	var treeDB bytes.Buffer

	So(tree.Serialise(&treeDB, dirRoot), ShouldBeNil)

	tr, err := tree.OpenMem(treeDB.Bytes())
	So(err, ShouldBeNil)

	return tr
}

func TestCreateRuleGroups(t *testing.T) {
	Convey("Given a plan database, you can create a slice of ruleGroups", t, func() {
		testDB, _ := plandb.PopulateExamplePlanDB(t)

		dirs := make(map[int64]*db.Directory)

		for dir := range testDB.ReadDirectories().Iter {
			dirs[dir.ID()] = dir
		}

		rgs, ruleList := createRuleGroups(testDB, dirs)
		So(len(rgs), ShouldEqual, 3)
		So(len(ruleList), ShouldEqual, 2)

		var rules []*db.Rule

		readRules := testDB.ReadRules()

		readRules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
			rules = append(rules, rule)

			return nil
		})

		So(rgs, ShouldResemble, []ruleGroup{
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/" + rules[0].Match),
				Group: rules[0],
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/" + rules[1].Match),
				Group: rules[1],
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/c/" + rules[2].Match),
				Group: rules[2],
			},
		})

		So(ruleList, ShouldResemble, map[string]struct{}{
			"/lustre/scratch123/humgen/a/b/": {},
			"/lustre/scratch123/humgen/a/c/": {},
		})
	})
}

func TestBackups(t *testing.T) {
	Convey("Given a plan database, a tree of wrstat info and an ibackup server", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

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

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := exampleTree()

		Convey("You can create ibackup sets for all automatic ibackup plans, excluding BackupNone and BackupManual", func() {
			setInfos, err := Backup(testDB, tr, ibackupClient)
			So(err, ShouldBeNil)
			So(setInfos, ShouldNotBeNil)
			So(len(setInfos), ShouldEqual, 1)
			So(setInfos[0].BackupSetName, ShouldEqual, "plan::/lustre/scratch123/humgen/a/b/")
			So(setInfos[0].Requestor, ShouldEqual, "userA")
			So(setInfos[0].FileCount, ShouldEqual, 2)

			sets, err := ibackupClient.GetSets("userA")
			So(err, ShouldBeNil)
			So(len(sets), ShouldEqual, 1)
			So(sets[0].Name, ShouldEqual, "plan::/lustre/scratch123/humgen/a/b/")
			So(sets[0].Requester, ShouldEqual, "userA")

			sets, err = ibackupClient.GetSets("userB")
			So(err, ShouldBeNil)
			So(len(sets), ShouldEqual, 0)
		})
	})
}
