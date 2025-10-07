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
	_ "modernc.org/sqlite"
	"vimagination.zapto.org/tree"
)

func TestFilePaths(t *testing.T) {
	Convey("Given a tree of wrstat info you can get all the absolute file paths", t, func() {
		tr := exampleTree()

		var paths []string

		filePaths(tr, func(path string) {
			paths = append(paths, path)
		})

		So(paths, ShouldResemble, []string{
			"/a/b/1.jpg",
			"/a/b/2.jpg",
			"/a/b/3.txt",
			"/a/b/temp.jpg",
			"/a/c/4.txt",
		})
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

		tr := exampleTree()

		Convey("You can create ibackup sets for all automatic ibackup plans, excluding BackupNone and BackupManual", func() {
			setIDs, err := Backup(testDB, tr, ibackupClient)
			So(err, ShouldBeNil)
			So(len(setIDs), ShouldEqual, 1)

			sets, err := ibackupClient.GetSets(userA)
			So(err, ShouldBeNil)
			So(len(sets), ShouldEqual, 1)
			So(sets[0].ID, ShouldEqual, setIDs[0])
			So(sets[0].Name, ShouldEqual, "plan::/a/b")
			So(sets[0].Requester, ShouldEqual, userA)
		})
	})
}
