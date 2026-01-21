package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ibackup_test "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"gopkg.in/yaml.v2"
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

type cmdConfig struct {
	IBackup        ibackup.Config
	ReportingRoots []string
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

		config := filepath.Join(t.TempDir(), "config.yaml")

		f, err := os.Create(config)
		So(err, ShouldBeNil)

		So(yaml.NewEncoder(f).Encode(&cmdConfig{
			IBackup: ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"": {
						Addr:  addr,
						Cert:  certPath,
						Token: filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
					},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^": {
						Transformer: "prefix=/:/remote/",
					},
				},
			},
		}), ShouldBeNil)

		So(f.Close(), ShouldBeNil)

		Convey("The backups command returns an error about required flags with no args", func() {
			out, err := exec.Command(appExe, "backup").CombinedOutput() //nolint:noctx
			So(err, ShouldNotBeNil)
			So(string(out), ShouldContainSubstring, "must be set when env")
		})

		Convey("The backups command results in a correct ibackup set being created given correct args", func() {
			_, dbPath := plandb.PopulateExamplePlanDB(t)

			out, err := exec.Command(appExe, "backup", "--plan", dbPath, //nolint:noctx
				"--tree", "testdata/tree.db", "--config", config).CombinedOutput()

			So(string(out), ShouldContainSubstring,
				"ibackup set 'plan::/lustre/scratch123/humgen/a/b/' created for userA with 2 files\n")
			So(err, ShouldBeNil)

			ibackupClient, err := ibackup.Connect(addr, certPath, "")
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
				"--tree", "testdata/tree.db", "--config", config).Run()

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
				"--config", config).CombinedOutput()
			So(
				string(out),
				ShouldContainSubstring,
				"ibackup set 'plan::/lustre/scratch123/humgen/a/b/' created for userA with 2 files\n",
			)
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
