package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal"
)

// in this go process we want to do the equivalent of typing `go build .` on the
// terminal which creates a binary called "backup-plans"
// so that we can then run do the equivalent of typing `./backup-plans backup
// --plan <planDB> --tree <treeDB> --ibackup <url> --cert <cert>`
// and check it works correctly by querying the ibackup server to see if the
// set got created with the expeted name and files in it.

const (
	app       = "backup-plans"
	userPerms = 0700
)

func TestMain(t *testing.T) {
	tmpDir, cleanup := buildSelf()
	if cleanup != nil {
		defer cleanup()
	}

	Convey("Given an ibackup test server", t, func() {
		So(tmpDir, ShouldNotBeEmpty)
		_, addr, certPath, dfn, err := internal.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		Convey("The backups command returns an error about required flags with no args", func() {
			out, err := exec.Command(filepath.Join(tmpDir, app), "backup").CombinedOutput()
			So(err, ShouldNotBeNil)
			So(string(out), ShouldContainSubstring, "required flag(s) \"cert\", \"ibackup\", \"plan\", \"tree\" not set")
		})

		Convey("The backups command results in a correct ibackup set being created given correct args", func() {
			out, err := exec.Command(filepath.Join(tmpDir, app), "backup", "--plan", "testdata/plan.db", "--tree", "testdata/tree.db", "--ibackup", addr, "--cert", certPath).CombinedOutput()

			So(string(out), ShouldEqual, "ibackup set 'plan::/lustre/scratch123/humgen/a/b/' created for userA with 2 files\n")
			So(err, ShouldBeNil)

			ibackupClient, err := ibackup.Connect(addr, certPath)
			So(err, ShouldBeNil)

			sets, err := ibackupClient.GetSets("userA")
			So(err, ShouldBeNil)
			So(sets, ShouldNotBeNil)
			So(len(sets), ShouldEqual, 1)
			So(sets[0].Name, ShouldEqual, "plan::/lustre/scratch123/humgen/a/b/")

			files, err := ibackupClient.GetFiles(sets[0].ID())
			So(err, ShouldBeNil)
			So(files, ShouldNotBeNil)
			So(len(files), ShouldEqual, 2)
			So(files[0].Path, ShouldEqual, "/lustre/scratch123/humgen/a/b/1.jpg")
			So(files[1].Path, ShouldEqual, "/lustre/scratch123/humgen/a/b/2.jpg")
		})
	})
}

func buildSelf() (string, func()) {
	//TODO: can we build in a tempdir instead with -o ...?
	tmpDir, err := os.MkdirTemp("", "backup-plans-test")
	if err != nil {
		failMainTest(err.Error())

		return "", nil
	}

	if err := exec.Command("go", "build", "-o", tmpDir).Run(); err != nil {
		failMainTest(err.Error())

		return "", nil
	}

	return tmpDir, func() { os.Remove(app) }
}

func failMainTest(err string) {
	fmt.Println(err) //nolint:forbidigo
}

// TODO: create main_test.go that builds our binary and tests
// literally running it with os/exec. We'll need a test tree.db and sqlite
// plan.db, plus environment variables for a test ibackup server and cert.
