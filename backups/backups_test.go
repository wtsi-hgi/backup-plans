package backups

import (
	"bytes"
	"os/user"
	"path/filepath"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	internal "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	_ "modernc.org/sqlite"
	"vimagination.zapto.org/tree"
)

func TestFileInfos(t *testing.T) {
	Convey("Given a tree of wrstat info you can get all the file infos", t, func() {
		tr := exampleTree()

		var paths []string

		sm, err := group.NewStatemachine([]group.PathGroup[int64]{
			{Path: []byte("*"), Group: &hasBackups},
		})
		So(err, ShouldBeNil)

		ctr := tr

		for _, part := range [...]string{"/", "lustre/", "scratch123/", "humgen/", "a/", "b/"} {
			ctr, err = ctr.(*tree.MemTree).Child(part) //nolint:errcheck,forcetypeassert
			So(err, ShouldBeNil)
		}

		figureOutFOFNs(ctr, sm.GetStateString("/lustre/scratch123/humgen/a/b/"),
			&summary.DirectoryPath{Name: "/lustre/scratch123/humgen/a/b/"}, func(path *summary.DirectoryPath, _ int64) {
				paths = append(paths, string(path.AppendTo(nil)))
			})

		So(len(paths), ShouldEqual, 5)

		So(paths, ShouldResemble, []string{
			"/lustre/scratch123/humgen/a/b/1.jpg",
			"/lustre/scratch123/humgen/a/b/2.jpg",
			"/lustre/scratch123/humgen/a/b/3.txt",
			"/lustre/scratch123/humgen/a/b/temp.jpg",
			"/lustre/scratch123/humgen/a/b/testdir/test.txt",
		})

		paths = paths[:0]

		figureOutFOFNs(tr, sm.GetStateString(""), nil, func(path *summary.DirectoryPath, _ int64) {
			paths = append(paths, string(path.AppendTo(nil)))
		})

		So(len(paths), ShouldEqual, 6)

		So(paths, ShouldResemble, []string{
			"/lustre/scratch123/humgen/a/b/1.jpg",
			"/lustre/scratch123/humgen/a/b/2.jpg",
			"/lustre/scratch123/humgen/a/b/3.txt",
			"/lustre/scratch123/humgen/a/b/temp.jpg",
			"/lustre/scratch123/humgen/a/b/testdir/test.txt",
			"/lustre/scratch123/humgen/a/c/4.txt",
		})
	})
}

func exampleTree() tree.Node { //nolint:ireturn,nolintlint
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
		mountpoint := "/"

		dirs, ruleList, err := readDirRules(testDB, mountpoint)
		So(err, ShouldBeNil)

		root := ruletree.NewRuleTree()

		for _, dr := range dirs {
			root.Set(dr.Path, dr.Rules, false)
		}

		root.Canon()
		root.MarkBackupDirs()

		rgs := collectRuleGroups(root, "/", nil)

		So(len(rgs), ShouldEqual, 9)
		So(len(ruleList), ShouldEqual, 3)

		var rules []*db.Rule

		readRules := testDB.ReadRules()

		readRules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
			rules = append(rules, rule)

			return nil
		})

		So(rgs, ShouldResemble, []group.PathGroup[int64]{
			{
				Path:  []byte("/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/scratch123/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/*/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/" + rules[0].Match),
				Group: ptr(rules[0].ID()),
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/" + rules[1].Match),
				Group: ptr(rules[1].ID()),
			},
		})
	})
}

func ptr[T any](n T) *T {
	return &n
}

func TestBackups(t *testing.T) {
	Convey("Given a plan database, a tree of wrstat info and an ibackup server", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		s, addr, certPath, dfn, err := internal.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		u, err := user.Current()
		So(err, ShouldBeNil)

		ibackupClient, err := ibackup.New(ibackup.Config{
			Servers: map[string]ibackup.ServerDetails{
				"server": {
					Addr:  addr,
					Cert:  certPath,
					Token: filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
				},
			},
			PathToServer: map[string]ibackup.ServerTransformer{
				"^/lustre/": {
					ServerName:  "server",
					Transformer: "prefix=/lustre/:/remote/",
				},
			},
		})
		So(err, ShouldBeNil)

		So(s, ShouldNotBeNil)
		So(u, ShouldNotBeNil)
		So(ibackupClient, ShouldNotBeNil)

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := exampleTree()

		Convey("You can create ibackup sets for all automatic ibackup plans, excluding BackupNone and manual backup types", func() { //nolint:lll
			setInfos, err := Backup(testDB, tr, ibackupClient)
			So(err, ShouldBeNil)
			So(setInfos, ShouldNotBeNil)
			So(len(setInfos), ShouldEqual, 1)
			So(setInfos[0].BackupSetName, ShouldEqual, "plan::/lustre/scratch123/humgen/a/b/")
			So(setInfos[0].Requestor, ShouldEqual, "userA")
			So(setInfos[0].FileCount, ShouldEqual, 2)

			sets, err := ibackupClient.GetBackupActivity("/lustre/scratch123/humgen/a/b/",
				"plan::/lustre/scratch123/humgen/a/b/", "userA")
			So(err, ShouldBeNil)
			So(sets.Name, ShouldEqual, "plan::/lustre/scratch123/humgen/a/b/")
			So(sets.Requester, ShouldEqual, "userA")

			sets, err = ibackupClient.GetBackupActivity("/lustre/scratch123/humgen/a/b/",
				"plan::/lustre/scratch123/humgen/a/b/", "userB")
			So(err, ShouldNotBeNil)
			So(sets, ShouldBeNil)
		})
	})
}
