package backups

import (
	"bytes"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	internal "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/ibackup/fofn"
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

func TestBackups(t *testing.T) {
	Convey("Given a plan database, a tree of wrstat info and an ibackup server", t, func() {
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

func TestBackupsC1(t *testing.T) {
	Convey("C1 acceptance tests for creating fofn files during backup", t, func() {
		Convey("1) API+fofn creates both ibackup set and fofn subdirectory", func() {
			s, addr, certPath, dfn, err := internal.NewTestIbackupServer(t)
			So(err, ShouldBeNil)

			Reset(func() { So(dfn(), ShouldBeNil) })

			fofnDir := t.TempDir()

			client, err := ibackup.New(ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"server": {
						Addr:    addr,
						Cert:    certPath,
						Token:   filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
						FofnDir: fofnDir,
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

			Reset(func() { client.Stop() })

			So(s, ShouldNotBeNil)

			testDB, _ := plandb.PopulateExamplePlanDB(t)
			setInfos, err := Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)
			So(setInfos, ShouldHaveLength, 1)
			So(setInfos[0].BackupSetName, ShouldEqual,
				"plan::/lustre/scratch123/humgen/a/b/")

			activity, err := client.GetBackupActivity("/lustre/scratch123/humgen/a/b/",
				"plan::/lustre/scratch123/humgen/a/b/", "userA")
			So(err, ShouldBeNil)
			So(activity, ShouldNotBeNil)

			subDir := filepath.Join(fofnDir,
				ibackup.SafeName("plan::/lustre/scratch123/humgen/a/b/"))

			fofnBytes, err := os.ReadFile(filepath.Join(subDir, "fofn"))
			So(err, ShouldBeNil)

			paths := strings.Split(strings.TrimSuffix(string(fofnBytes), "\x00"), "\x00")
			So(paths, ShouldHaveLength, 2)
			So(slices.Contains(paths, "/lustre/scratch123/humgen/a/b/1.jpg"), ShouldBeTrue)
			So(slices.Contains(paths, "/lustre/scratch123/humgen/a/b/2.jpg"), ShouldBeTrue)

			cfg, err := fofn.ReadConfig(subDir)
			So(err, ShouldBeNil)
			So(cfg.Metadata["requestor"], ShouldEqual, "userA")
			So(cfg.Metadata["review"], ShouldNotBeBlank)
			So(cfg.Metadata["remove"], ShouldNotBeBlank)
		})

		Convey("2) fofndir-only server writes fofn and skips API path safely", func() {
			fofnDir := t.TempDir()

			client, err := ibackup.New(ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"fofn_only": {FofnDir: fofnDir},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/lustre/": {
						ServerName:  "fofn_only",
						Transformer: "prefix=/lustre/:/remote/",
					},
				},
			})
			So(err, ShouldBeNil)
			Reset(func() { client.Stop() })

			testDB, _ := plandb.PopulateExamplePlanDB(t)
			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			_, err = os.Stat(filepath.Join(fofnDir,
				ibackup.SafeName("plan::/lustre/scratch123/humgen/a/b/"), "fofn"))
			So(err, ShouldBeNil)
		})

		Convey("3) API-only path preserves existing behaviour and no fofn is created", func() {
			_, addr, certPath, dfn, err := internal.NewTestIbackupServer(t)
			So(err, ShouldBeNil)

			Reset(func() { So(dfn(), ShouldBeNil) })

			unusedFofnDir := t.TempDir()

			client, err := ibackup.New(ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"api": {
						Addr:  addr,
						Cert:  certPath,
						Token: filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
					},
					"unused_fofn": {FofnDir: unusedFofnDir},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/lustre/": {
						ServerName:  "api",
						Transformer: "prefix=/lustre/:/remote/",
					},
					"^/not-used/": {
						ServerName:  "unused_fofn",
						Transformer: "prefix=/not-used/:/remote/",
					},
				},
			})
			So(err, ShouldBeNil)
			Reset(func() { client.Stop() })

			testDB, _ := plandb.PopulateExamplePlanDB(t)
			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			activity, err := client.GetBackupActivity("/lustre/scratch123/humgen/a/b/",
				"plan::/lustre/scratch123/humgen/a/b/", "userA")
			So(err, ShouldBeNil)
			So(activity, ShouldNotBeNil)

			entries, err := os.ReadDir(unusedFofnDir)
			So(err, ShouldBeNil)
			So(entries, ShouldBeEmpty)
		})

		Convey("4) frequency 0 writes freeze=true in config.yml", func() {
			fofnDir := t.TempDir()

			client, err := ibackup.New(ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"fofn_only": {FofnDir: fofnDir},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/lustre/": {
						ServerName:  "fofn_only",
						Transformer: "prefix=/lustre/:/remote/",
					},
				},
			})
			So(err, ShouldBeNil)
			Reset(func() { client.Stop() })

			testDB, _ := plandb.CreateTestDatabase(t)
			createIBackupDirectory(testDB, "/lustre/scratch123/humgen/a/b/", "userA", 0)

			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			subDir := filepath.Join(fofnDir,
				ibackup.SafeName("plan::/lustre/scratch123/humgen/a/b/"))
			cfg, err := fofn.ReadConfig(subDir)
			So(err, ShouldBeNil)
			So(cfg.Freeze, ShouldBeTrue)
		})

		Convey("5) different paths route fofn outputs to the correct fofndir", func() {
			fofnDirOne := t.TempDir()
			fofnDirTwo := t.TempDir()

			client, err := ibackup.New(ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"one": {FofnDir: fofnDirOne},
					"two": {FofnDir: fofnDirTwo},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/lustre/scratch123/humgen/a/": {
						ServerName:  "one",
						Transformer: "prefix=/lustre/:/remote/one/",
					},
					"^/lustre/scratch123/humgen/b/": {
						ServerName:  "two",
						Transformer: "prefix=/lustre/:/remote/two/",
					},
				},
			})
			So(err, ShouldBeNil)
			Reset(func() { client.Stop() })

			testDB, _ := plandb.CreateTestDatabase(t)
			createIBackupDirectory(testDB, "/lustre/scratch123/humgen/a/b/", "userA", 1)
			createIBackupDirectory(testDB, "/lustre/scratch123/humgen/b/", "userB", 1)

			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			_, err = os.Stat(filepath.Join(fofnDirOne,
				ibackup.SafeName("plan::/lustre/scratch123/humgen/a/b/"), "fofn"))
			So(err, ShouldBeNil)

			_, err = os.Stat(filepath.Join(fofnDirTwo,
				ibackup.SafeName("plan::/lustre/scratch123/humgen/b/"), "fofn"))
			So(err, ShouldBeNil)
		})

		Convey("6) requestor change updates config without rewriting gated fofn", func() {
			fofnDir := t.TempDir()

			client, err := ibackup.New(ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"fofn_only": {FofnDir: fofnDir},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/lustre/": {
						ServerName:  "fofn_only",
						Transformer: "prefix=/lustre/:/remote/",
					},
				},
			})
			So(err, ShouldBeNil)
			Reset(func() { client.Stop() })

			testDB, _ := plandb.CreateTestDatabase(t)
			dir := createIBackupDirectory(testDB, "/lustre/scratch123/humgen/a/b/", "userA", 7)

			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			setName := "plan::/lustre/scratch123/humgen/a/b/"
			fofnPath := filepath.Join(fofnDir, ibackup.SafeName(setName), "fofn")

			before, err := os.Stat(fofnPath)
			So(err, ShouldBeNil)

			dir.ClaimedBy = "userB"
			So(testDB.UpdateDirectory(dir), ShouldBeNil)

			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			after, err := os.Stat(fofnPath)
			So(err, ShouldBeNil)
			So(after.ModTime(), ShouldEqual, before.ModTime())

			cfg, err := fofn.ReadConfig(filepath.Join(fofnDir, ibackup.SafeName(setName)))
			So(err, ShouldBeNil)
			So(cfg.Metadata["requestor"], ShouldEqual, "userB")
		})

		Convey("7) unclaimed directories do not delete existing fofn subdirectories", func() {
			fofnDir := t.TempDir()

			client, err := ibackup.New(ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"fofn_only": {FofnDir: fofnDir},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/lustre/": {
						ServerName:  "fofn_only",
						Transformer: "prefix=/lustre/:/remote/",
					},
				},
			})
			So(err, ShouldBeNil)
			Reset(func() { client.Stop() })

			testDB, _ := plandb.CreateTestDatabase(t)
			dir := createIBackupDirectory(testDB, "/lustre/scratch123/humgen/a/b/", "userA", 1)

			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			setName := "plan::/lustre/scratch123/humgen/a/b/"
			subDir := filepath.Join(fofnDir, ibackup.SafeName(setName))

			_, err = os.Stat(filepath.Join(subDir, "fofn"))
			So(err, ShouldBeNil)

			So(testDB.RemoveDirectory(dir), ShouldBeNil)

			_, err = Backup(testDB, exampleTree(), client)
			So(err, ShouldBeNil)

			_, err = os.Stat(filepath.Join(subDir, "fofn"))
			So(err, ShouldBeNil)
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

func createIBackupDirectory(testDB *db.DB, path, claimedBy string, frequency uint) *db.Directory {
	dir := &db.Directory{
		Path:       path,
		ClaimedBy:  claimedBy,
		Frequency:  frequency,
		ReviewDate: 1735689600,
		RemoveDate: 1767225600,
	}

	So(testDB.CreateDirectory(dir), ShouldBeNil)
	So(testDB.CreateDirectoryRule(dir, &db.Rule{BackupType: db.BackupIBackup, Match: "*"}), ShouldBeNil)

	return dir
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
