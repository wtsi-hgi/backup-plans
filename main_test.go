package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ibackup_test "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/ibackup/fofn"
	"gopkg.in/yaml.v2"
	"vimagination.zapto.org/tree"
)

const app = "backup-plans"

var appExe string //nolint:gochecknoglobals

type cmdConfig struct {
	IBackup        ibackup.Config
	ReportingRoots []string
	MainProgrammes []string
}

func TestCommands(t *testing.T) {
	mysqlConnection := os.Getenv("BACKUP_PLANS_CONNECTION_TEST")

	os.Unsetenv("BACKUP_PLANS_CONNECTION_TEST")

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

func TestCommandsE1(t *testing.T) {
	Convey("E1 integration acceptance tests for end-to-end fofn support", t, func() {
		So(appExe, ShouldNotBeEmpty)

		writeConfig := func(cfg cmdConfig) string {
			configPath := filepath.Join(t.TempDir(), "config.yaml")

			f, err := os.Create(configPath)
			So(err, ShouldBeNil)
			So(yaml.NewEncoder(f).Encode(&cfg), ShouldBeNil)
			So(f.Close(), ShouldBeNil)

			return configPath
		}

		Convey("1) API+fofndir backup creates ibackup set and fofn files", func() {
			_, addr, certPath, dfn, err := ibackup_test.NewTestIbackupServer(t)
			So(err, ShouldBeNil)
			Reset(func() { So(dfn(), ShouldBeNil) })

			fofnDir := t.TempDir()
			configPath := writeConfig(cmdConfig{
				IBackup: ibackup.Config{
					Servers: map[string]ibackup.ServerDetails{
						"s1": {
							Addr:    addr,
							Cert:    certPath,
							Token:   filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
							FofnDir: fofnDir,
						},
					},
					PathToServer: map[string]ibackup.ServerTransformer{
						"^": {
							ServerName:  "s1",
							Transformer: "prefix=/:/remote/",
						},
					},
				},
			})

			_, dbPath := plandb.PopulateExamplePlanDB(t)

			out, err := exec.Command(appExe, "backup", "--plan", dbPath, //nolint:noctx
				"--tree", "testdata/tree.db", "--config", configPath).CombinedOutput()
			So(err, ShouldBeNil)
			So(string(out), ShouldContainSubstring,
				"ibackup set 'plan::/lustre/scratch123/humgen/a/b/' created for userA with 2 files\n")

			ibackupClient, err := ibackup.Connect(addr, certPath, "")
			So(err, ShouldBeNil)

			sets, err := ibackupClient.GetSets("userA")
			So(err, ShouldBeNil)
			So(sets, ShouldHaveLength, 1)
			So(sets[0].Name, ShouldEqual, "plan::/lustre/scratch123/humgen/a/b/")

			subDir := filepath.Join(fofnDir,
				ibackup.SafeName("plan::/lustre/scratch123/humgen/a/b/"))
			fofnPath := filepath.Join(subDir, "fofn")

			fofnBytes, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(strings.HasSuffix(string(fofnBytes), "\x00"), ShouldBeTrue)

			paths := strings.Split(strings.TrimSuffix(string(fofnBytes), "\x00"), "\x00")
			So(paths, ShouldHaveLength, 2)
			So(slices.Contains(paths, "/lustre/scratch123/humgen/a/b/1.jpg"), ShouldBeTrue)
			So(slices.Contains(paths, "/lustre/scratch123/humgen/a/b/2.jpg"), ShouldBeTrue)

			cfg, err := fofn.ReadConfig(subDir)
			So(err, ShouldBeNil)
			So(cfg.Transformer, ShouldEqual, "prefix=/:/remote/")
			So(cfg.Freeze, ShouldBeFalse)
			So(cfg.Metadata["requestor"], ShouldEqual, "userA")
			So(cfg.Metadata["review"], ShouldNotBeBlank)
			So(cfg.Metadata["remove"], ShouldNotBeBlank)
		})

		Convey("2) fofndir-only backup succeeds and still creates fofn", func() {
			fofnDir := t.TempDir()
			configPath := writeConfig(cmdConfig{
				IBackup: ibackup.Config{
					Servers: map[string]ibackup.ServerDetails{
						"fofn_only": {FofnDir: fofnDir},
					},
					PathToServer: map[string]ibackup.ServerTransformer{
						"^": {
							ServerName:  "fofn_only",
							Transformer: "prefix=/:/remote/",
						},
					},
				},
			})

			_, dbPath := plandb.PopulateExamplePlanDB(t)

			out, err := exec.Command(appExe, "backup", "--plan", dbPath, //nolint:noctx
				"--tree", "testdata/tree.db", "--config", configPath).CombinedOutput()
			So(err, ShouldBeNil)
			So(string(out), ShouldContainSubstring,
				"ibackup set 'plan::/lustre/scratch123/humgen/a/b/' created for userA with 2 files\n")

			_, statErr := os.Stat(filepath.Join(fofnDir,
				ibackup.SafeName("plan::/lustre/scratch123/humgen/a/b/"), "fofn"))
			So(statErr, ShouldBeNil)
		})

		Convey("3) summary endpoint includes fofn:<dir> status from pre-created status file", func() {
			fofnDir := t.TempDir()
			treePath := filepath.Join(t.TempDir(), "tree.db")

			treeFile, err := os.Create(treePath)
			So(err, ShouldBeNil)
			So(tree.Serialise(treeFile, plandb.ExampleTree()), ShouldBeNil)
			So(treeFile.Close(), ShouldBeNil)

			configPath := writeConfig(cmdConfig{
				IBackup: ibackup.Config{
					Servers: map[string]ibackup.ServerDetails{
						"fofn_only": {FofnDir: fofnDir},
					},
					PathToServer: map[string]ibackup.ServerTransformer{
						"^": {
							ServerName:  "fofn_only",
							Transformer: "prefix=/:/remote/",
						},
					},
				},
				ReportingRoots: []string{"/lustre/"},
			})

			_, dbPath := plandb.PopulateExamplePlanDB(t)

			backupOut, err := exec.Command(appExe, "backup", "--plan", dbPath, //nolint:noctx
				"--tree", treePath, "--config", configPath).CombinedOutput()
			So(err, ShouldBeNil)
			So(string(backupOut), ShouldContainSubstring,
				"ibackup set 'plan::/lustre/scratch123/humgen/a/b/' created for userA with 2 files\n")

			setName := "plan::/lustre/scratch123/humgen/a/b/"
			statusPath := filepath.Join(fofnDir, ibackup.SafeName(setName), "status")
			statusContent := "SUMMARY\tuploaded=3\treplaced=1\tunmodified=4\tmissing=5\t" +
				"failed=2\tfrozen=6\torphaned=7\twarning=8\thardlink=9\tnot_processed=0\n"
			So(os.WriteFile(statusPath, []byte(statusContent), 0o600), ShouldBeNil)

			l, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx
			So(err, ShouldBeNil)

			tcpAddr, ok := l.Addr().(*net.TCPAddr)
			So(ok, ShouldBeTrue)

			port := tcpAddr.Port
			err = l.Close()
			So(err, ShouldBeNil)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cmd := exec.CommandContext(ctx, appExe, "server",
				"--plan", dbPath,
				"--config", configPath,
				"--listen", strconv.Itoa(port),
				treePath)

			var serverOutput bytes.Buffer

			cmd.Stdout = &serverOutput
			cmd.Stderr = &serverOutput

			So(cmd.Start(), ShouldBeNil)

			waitErr := make(chan error, 1)

			go func() {
				waitErr <- cmd.Wait()
			}()

			defer func() {
				killErr := cmd.Process.Kill()
				So(killErr == nil || errors.Is(killErr, os.ErrProcessDone), ShouldBeTrue)

				select {
				case <-waitErr:
				default:
				}
			}()

			baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
			client := &http.Client{Timeout: time.Second}

			var resp *http.Response

			deadline := time.Now().Add(5 * time.Second)

		pollLoop:
			for time.Now().Before(deadline) {
				select {
				case wait := <-waitErr:
					err = fmt.Errorf("server exited early: %w; output: %s", wait, serverOutput.String())

					break pollLoop
				default:
				}

				resp, err = client.Get(baseURL + "/api/report/summary") //nolint:noctx
				if err == nil {
					break pollLoop
				}

				time.Sleep(100 * time.Millisecond)
			}

			So(err, ShouldBeNil)

			So(resp, ShouldNotBeNil)
			defer resp.Body.Close()

			So(resp.StatusCode, ShouldEqual, http.StatusOK)

			var summary struct {
				BackupStatus map[string]ibackup.SetBackupActivity `json:"BackupStatus"`
			}

			So(json.NewDecoder(resp.Body).Decode(&summary), ShouldBeNil)

			entry, ok := summary.BackupStatus["fofn:/lustre/scratch123/humgen/a/b/"]
			So(ok, ShouldBeTrue)
			So(entry.Uploaded, ShouldEqual, 3)
			So(entry.Replaced, ShouldEqual, 1)
			So(entry.Unmodified, ShouldEqual, 4)
			So(entry.Missing, ShouldEqual, 5)
			So(entry.Failures, ShouldEqual, 2)
			So(entry.Frozen, ShouldEqual, 6)
			So(entry.Orphaned, ShouldEqual, 7)
			So(entry.Warning, ShouldEqual, 8)
			So(entry.Hardlink, ShouldEqual, 9)
		})
	})
}

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
