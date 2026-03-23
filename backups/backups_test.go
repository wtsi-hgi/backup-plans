package backups

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/ibackup/fofn"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	_ "modernc.org/sqlite"
	"vimagination.zapto.org/tree"
)

func TestFileInfos(t *testing.T) {
	Convey("Given a tree of wrstat info you can get all the file infos", t, func() {
		tr := exampleTree()

		var paths []string

		sm, err := ruletree.BuildMultiStateMachine([]ruletree.Rules{
			{{Path: []byte("*"), Group: &hasBackups}},
		})
		So(err, ShouldBeNil)

		ctr := tr

		for _, part := range [...]string{"/", "lustre/", "scratch123/", "humgen/", "a/", "b/"} {
			ctr, err = ctr.(*tree.MemTree).Child(part) //nolint:errcheck,forcetypeassert
			So(err, ShouldBeNil)
		}

		figureOutFOFNs(ctr, sm,
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

		So(len(paths), ShouldEqual, 8)

		So(paths, ShouldResemble, []string{
			"/lustre/scratch123/humgen/a/b/1.jpg",
			"/lustre/scratch123/humgen/a/b/2.jpg",
			"/lustre/scratch123/humgen/a/b/3.txt",
			"/lustre/scratch123/humgen/a/b/temp.jpg",
			"/lustre/scratch123/humgen/a/b/testdir/test.txt",
			"/lustre/scratch123/humgen/a/c/4.txt",
			"/lustre/scratch123/humgen/b/5.txt",
			"/lustre/scratch123/humgen/b/c/6.txt",
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

	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/b/5.txt", 2, 1, 6, 12346)
	directories.AddFile(&dirRoot.Directory, "lustre/scratch123/humgen/b/c/6.txt", 2, 1, 6, 12346)

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

		rules := root.BuildRules()

		rules[0] = collectRuleGroups(root, "/", rules[0])

		So(len(rules), ShouldEqual, 1)
		So(len(rules[0]), ShouldEqual, 10)
		So(len(ruleList), ShouldEqual, 3)

		readRules := testDB.ReadRules()

		var dbRules []*db.Rule

		readRules.ForEach(func(rule *db.Rule) error { //nolint:errcheck
			dbRules = append(dbRules, rule)

			return nil
		})

		slices.SortFunc(rules[0], func(a, b group.PathGroup[int64]) int {
			return bytes.Compare(a.Path, b.Path)
		})

		So(rules[0], ShouldResemble, ruletree.Rules{
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
				Path:  []byte("/lustre/scratch123/humgen/a/b/" + dbRules[0].Match),
				Group: ptr(dbRules[0].ID()),
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/*/"),
				Group: &hasBackups,
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/b/" + dbRules[1].Match),
				Group: ptr(dbRules[1].ID()),
			},
			{
				Path:  []byte("/lustre/scratch123/humgen/a/c/" + dbRules[2].Match),
				Group: ptr(dbRules[2].ID()),
			},
		})
	})
}

func ptr[T any](n T) *T {
	return &n
}

func TestBackups(t *testing.T) {
	Convey("Given a plan database, a tree of wrstat info and an ibackup server", t, func() {
		fofnDir := t.TempDir()

		ibackupClient, err := ibackup.New(ibackup.Config{
			Servers: map[string]ibackup.ServerDetails{
				"server": {
					FOFNDir: fofnDir,
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
				"plan::/lustre/scratch123/humgen/a/b/", "userA", false)
			So(err, ShouldBeNil)
			So(sets.Name, ShouldEqual, "plan::/lustre/scratch123/humgen/a/b/")
			So(sets.Requester, ShouldEqual, "userA")

			sets, err = ibackupClient.GetBackupActivity("/lustre/scratch123/humgen/a/b/",
				"plan::/lustre/scratch123/humgen/a/b/", "userB", false)
			So(err, ShouldNotBeNil)
			So(sets, ShouldBeNil)
		})

		Convey("A FOFN backed up directory should not include files in a directory marked NoBackup", func() {
			testDB, _ = plandb.CreateTestDatabase(t)

			dirB := &db.Directory{
				Path:      "/lustre/scratch123/humgen/b/",
				ClaimedBy: "userA",
				Frequency: 1,
			}
			dirC := &db.Directory{
				Path:      "/lustre/scratch123/humgen/b/c/",
				ClaimedBy: "userA",
				Frequency: 1,
			}

			So(testDB.CreateDirectory(dirB), ShouldBeNil)
			So(testDB.CreateDirectory(dirC), ShouldBeNil)

			ruleB := &db.Rule{
				BackupType: db.BackupIBackup,
				Match:      "*",
			}
			ruleC := &db.Rule{
				BackupType: db.BackupNone,
				Match:      "*",
			}

			So(testDB.CreateDirectoryRule(dirB, ruleB), ShouldBeNil)
			So(testDB.CreateDirectoryRule(dirC, ruleC), ShouldBeNil)

			setInfos, err := Backup(testDB, tr, ibackupClient)
			So(err, ShouldBeNil)
			So(setInfos, ShouldNotBeNil)
			So(len(setInfos), ShouldEqual, 1)
			So(setInfos[0].BackupSetName, ShouldEqual, "plan::/lustre/scratch123/humgen/b/")
			So(setInfos[0].Requestor, ShouldEqual, "userA")
			So(setInfos[0].FileCount, ShouldEqual, 1)
		})
	})
}

func TestAddFofnsToIBackup(t *testing.T) {
	Convey("A set with Frozen=true can temporarily disable the frozen status when Unfreeze is set", t, func() {
		fofnDir := t.TempDir()

		ibackupClient, err := ibackup.New(ibackup.Config{
			Servers: map[string]ibackup.ServerDetails{
				"server": {
					FOFNDir: fofnDir,
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
		So(ibackupClient, ShouldNotBeNil)

		now := time.Now().Truncate(time.Second)

		for _, path := range [...]string{
			"/lustre/a",
			"/lustre/b",
			"/lustre/c",
			"/lustre/d",
		} {
			setDir := filepath.Join(fofnDir, (&set.Set{Requester: "a", Name: setNamePrefix + path}).ID())
			statusFile := filepath.Join(setDir, "status")

			So(os.MkdirAll(setDir, 0700), ShouldBeNil)
			So(fofn.WriteConfig(setDir, fofn.SubDirConfig{
				Transformer: "prefix=/lustre/:/remote/",
				Requester:   "a",
				Name:        setNamePrefix + path,
			}), ShouldBeNil)
			So(os.WriteFile(statusFile, nil, 0600), ShouldBeNil)
		}

		ft := make(frozenTest)

		_, err = addFofnsToIBackup(clientWrapper{ibackupClient, ft}, map[*db.Directory][]string{
			{ClaimedBy: "a", Path: "/lustre/a"}:                                                 {},
			{ClaimedBy: "a", Path: "/lustre/b", Melt: now.Add(time.Hour).Unix()}:                {},
			{ClaimedBy: "a", Path: "/lustre/c", Frozen: true, Melt: now.Add(time.Hour).Unix()}:  {},
			{ClaimedBy: "a", Path: "/lustre/d", Frozen: true, Melt: now.Add(-time.Hour).Unix()}: {},
		})
		So(err, ShouldBeNil)

		So(ft, ShouldResemble, frozenTest{
			"/lustre/a": false,
			"/lustre/b": false,
			"/lustre/c": true,
			"/lustre/d": false,
		})
	})
}

type justGet interface {
	GetBackupActivity(path, setName, requester string, manual bool) (*ibackup.SetBackupActivity, error)
}

type clientWrapper struct {
	justGet
	frozenTest
}

type frozenTest map[string]bool

func (f frozenTest) Backup(path, _, _ string, _ []string, _ int, frozen bool, _, _ int64) error { //nolint:unparam
	f[path] = frozen

	return nil
}
