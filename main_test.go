package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ibackup_test "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"github.com/wtsi-hgi/ibackup/set"
	"vimagination.zapto.org/tree"
)

const app = "backup-plans"

var appExe string //nolint:gochecknoglobals

func TestMain(m *testing.M) {
	tmpDir, cleanup, err := buildSelf()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if cleanup != nil {
		defer cleanup()
	}

	appExe = filepath.Join(tmpDir, app)

	m.Run()
}

func TestCommands(t *testing.T) {
	mysqlConnection := os.Getenv("BACKUP_PLANS_CONNECTION_TEST")
	os.Unsetenv("BACKUP_PLANS_CONNECTION_TEST")

	if err := testirods.AddPseudoIRODsToolsToPathIfRequired(t); err != nil {
		t.Fatal(err)
	}

	Convey("Given an ibackup test server", t, func() {
		So(appExe, ShouldNotBeEmpty)

		_, addr, certPath, dfn, err := ibackup_test.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		Convey("The backups command returns an error about required flags with no args", func() {
			out, err := exec.Command(appExe, "backup").CombinedOutput() //nolint:noctx
			So(err, ShouldNotBeNil)
			So(string(out), ShouldContainSubstring, "must be set when env")
		})

		Convey("The backups command results in a correct ibackup set being created given correct args", func() {
			_, dbPath := plandb.PopulateExamplePlanDB(t)

			out, err := exec.Command(appExe, "backup", "--plan", dbPath, //nolint:noctx
				"--tree", "testdata/tree.db", "--ibackup", addr, "--cert", certPath).CombinedOutput()

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

		Convey("The backups command fails with an invalid plan schema", func() {
			_, dbPath := plandb.PopulateExamplePlanDB(t)
			_, err := exec.Command(appExe, "backup", "--plan", "bad:"+dbPath, //nolint:noctx
				"--tree", "testdata/tree.db", "--ibackup", addr, "--cert", certPath).CombinedOutput()
			So(err, ShouldNotBeNil)
		})

		Convey("The backups command works with an explicit sqlite3 plan schema", func() {
			_, dbPath := plandb.PopulateExamplePlanDB(t)
			err := exec.Command(appExe, "backup", "--plan", dbPath, //nolint:noctx
				"--tree", "testdata/tree.db", "--ibackup", addr, "--cert", certPath).Run()

			So(err, ShouldBeNil)
		})

		if mysqlConnection == "" {
			SkipConvey("Skipping mysql tests as BACKUP_PLANS_CONNECTION_TEST not set", func() {})

			return
		}

		Convey("The backups command works with a mysql plan database", func() {
			os.Setenv("BACKUP_PLANS_CONNECTION_TEST", mysqlConnection)
			plandb.PopulateExamplePlanDB(t)

			out, err := exec.Command( //nolint:noctx
				appExe, "backup", "--plan",
				mysqlConnection, "--tree", "testdata/tree.db",
				"--ibackup", addr, "--cert", certPath).CombinedOutput()
			So(string(out), ShouldEqual, "ibackup set 'plan::/lustre/scratch123/humgen/a/b/' created for userA with 2 files\n")
			So(err, ShouldBeNil)
		})
	})
}

func buildSelf() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "backup-plans-test")
	if err != nil {
		return "", nil, err
	}

	if err := exec.Command("go", "build", "-tags", "dev", "-o", tmpDir).Run(); err != nil { //nolint:noctx
		return "", nil, err
	}

	return tmpDir, func() { os.Remove(app) }, nil
}

func TestFrontend(t *testing.T) {
	frontendPort := os.Getenv("BACKUP_PLANS_TEST_FRONTEND")
	if frontendPort == "" {
		SkipConvey("Frontend", t, func() {})

		return
	}

	Convey("Frontend", t, func() {
		_, addr, certPath, dfn, err := ibackup_test.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { dfn() })

		client, err := ibackup.Connect(addr, certPath)
		So(err, ShouldBeNil)

		exampleSet := &set.Set{
			Name:        "plan::/lustre/scratch123/humgen/a/c/",
			Requester:   "userB",
			Transformer: "humgen",
		}

		err = client.AddOrUpdateSet(exampleSet)
		So(err, ShouldBeNil)

		exampleSet2 := &set.Set{
			Name:        "plan::/lustre/scratch123/humgen/a/c/newdir/",
			Requester:   "userC",
			Transformer: "humgen",
		}

		err = client.AddOrUpdateSet(exampleSet2)
		So(err, ShouldBeNil)

		err = client.TriggerDiscovery(exampleSet.ID(), false)
		So(err, ShouldBeNil)

		err = client.TriggerDiscovery(exampleSet2.ID(), false)
		So(err, ShouldBeNil)

		_, dbPath := plandb.FofnTestDB(t)
		treeDB := plandb.FofnTestTree()

		treePath := filepath.Join(t.TempDir(), "tree.db")

		f, err := os.Create(treePath)
		So(err, ShouldBeNil)

		err = tree.Serialise(f, treeDB)
		So(err, ShouldBeNil)

		err = f.Close()
		So(err, ShouldBeNil)

		cwd, err := os.Getwd()
		So(err, ShouldBeNil)

		cmd := exec.Command( //nolint:noctx
			appExe, "server", "--plan",
			dbPath, "--ibackup", addr, "--cert", certPath,
			"--report", "/lustre/scratch123/humgen/a/b/",
			"--report", "/lustre/scratch123/humgen/a/c/",
			"--report", "/lustre/scratch123/humgen/a/d/",
			"--listen", frontendPort, treePath)

		cmd.Dir = filepath.Join(cwd, "frontend")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		So(cmd.Start(), ShouldBeNil)

		c := make(chan os.Signal, 1)
		d := make(chan struct{}, 1)

		signal.Notify(c, os.Interrupt)

		go func() {
			select {
			case <-d:
			case <-c:
				cmd.Process.Kill() //
			}
		}()

		So(cmd.Wait(), ShouldBeNil)

		close(d)
	})
}
