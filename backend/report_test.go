package backend

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	appconfig "github.com/wtsi-hgi/backup-plans/config"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	internalconfig "github.com/wtsi-hgi/backup-plans/internal/config"
	internalibackup "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"vimagination.zapto.org/tree"
)

var errTimeoutWaitingForSuccessfulBackup = errors.New("timeout waiting for successful backup status")

func TestReport(t *testing.T) {
	Convey("Given a test db, tree, roots and ibackup client, a backup can be made", t, func() {
		testDB, _ := plandb.PopulateBigExamplePlanDB(t)
		testTree := plandb.ExampleTreeBig()
		path := filepath.Join(t.TempDir(), "testdb")
		file, err := os.Create(path)
		So(err, ShouldBeNil)
		err = tree.Serialise(file, testTree)
		So(err, ShouldBeNil)
		err = file.Close()
		So(err, ShouldBeNil)

		firstUser, err := user.LookupId("1")
		So(err, ShouldBeNil)

		secondUser, err := user.LookupId("2")
		So(err, ShouldBeNil)

		firstGroup, err := user.LookupGroupId("1")
		So(err, ShouldBeNil)

		secondGroup, err := user.LookupGroupId("2")
		So(err, ShouldBeNil)

		roots := []string{
			"/lustre/scratch123/humgen/a/[bc]/",
		}

		srv, err := New(
			testDB,
			func(_ *http.Request) string { return "test" },
			internalconfig.NewConfig(t, nil, nil, roots, 0),
		)
		So(err, ShouldBeNil)
		err = srv.AddTree(path)
		So(err, ShouldBeNil)

		exampleSet := &set.Set{
			Name:        "plan::/lustre/scratch123/humgen/a/c/",
			Requester:   "userB",
			Transformer: "prefix=/:/remote/",
		}

		single := getSingleClientFromMultiClient(t, srv.config.GetIBackupClient())

		err = single.AddOrUpdateSet(exampleSet)
		So(err, ShouldBeNil)

		beforeTrigger := time.Now()

		err = single.TriggerDiscovery(exampleSet.ID(), false)
		So(err, ShouldBeNil)
		Convey("A summary can be retrieved", func() {
			gotSummary, err := awaitSummaryWithSuccessfulBackup(srv,
				"/lustre/scratch123/humgen/a/c/", beforeTrigger)
			So(err, ShouldBeNil)

			rules := slices.Collect(testDB.ReadRules().Iter)

			Convey("Containing the correctly updated backup activity", func() {
				expectedSummary := summary{
					Summaries: map[string]*ruletree.DirSummary{
						"/lustre/scratch123/humgen/a/b/": {
							ClaimedBy: "userA",
							User:      firstUser.Username,
							Group:     firstGroup.Name,
							RuleSummaries: []ruletree.Rule{
								{
									ID: 0,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98767, Files: 2, Size: 17},
										{Name: users.Username(2), MTime: 12346, Files: 1, Size: 6},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(1), MTime: 98766, Files: 2, Size: 15},
										{Name: users.Group(2), MTime: 98767, Files: 1, Size: 8},
									},
								},
								{
									ID: 1,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98767, Files: 2, Size: 17},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(1), MTime: 98766, Files: 1, Size: 9},
										{Name: users.Group(2), MTime: 98767, Files: 1, Size: 8},
									},
								},
								{
									ID: 2,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98767, Files: 1, Size: 8},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(2), MTime: 98767, Files: 1, Size: 8},
									},
								},
								{
									ID: 4,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98766, Files: 1, Size: 9},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(1), MTime: 98766, Files: 1, Size: 9},
									},
								},
								{
									ID: 5,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98766, Files: 1, Size: 9},
										{Name: users.Username(2), MTime: 12346, Files: 1, Size: 6},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(1), MTime: 98766, Files: 2, Size: 15},
									},
								},
							},
							Children: map[string]*ruletree.DirSummary{
								"/lustre/scratch123/humgen/a/b/newdir/": {
									ClaimedBy: "userC",
									User:      firstUser.Username,
									Group:     firstGroup.Name,
									RuleSummaries: []ruletree.Rule{
										{
											ID: 4,
											Users: ruletree.RuleStats{
												{Name: users.Username(1), MTime: 98766, Files: 1, Size: 9},
											},
											Groups: ruletree.RuleStats{
												{Name: users.Group(1), MTime: 98766, Files: 1, Size: 9},
											},
										},
										{
											ID: 5,
											Users: ruletree.RuleStats{
												{Name: users.Username(1), MTime: 98766, Files: 1, Size: 9},
												{Name: users.Username(2), MTime: 12346, Files: 1, Size: 6},
											},
											Groups: ruletree.RuleStats{
												{Name: users.Group(1), MTime: 98766, Files: 2, Size: 15},
											},
										},
									},
									Children: map[string]*ruletree.DirSummary{},
								},
								"/lustre/scratch123/humgen/a/b/newdir/testextradir/": {
									ClaimedBy: "userD",
									User:      firstUser.Username,
									Group:     firstGroup.Name,
									RuleSummaries: []ruletree.Rule{
										{
											ID: 5,
											Users: ruletree.RuleStats{
												{Name: users.Username(2), MTime: 12346, Files: 1, Size: 6},
											},
											Groups: ruletree.RuleStats{
												{Name: users.Group(1), MTime: 12346, Files: 1, Size: 6},
											},
										},
									},
									Children: map[string]*ruletree.DirSummary{},
								},
							},
						},
						"/lustre/scratch123/humgen/a/c/": {
							ClaimedBy: "userB",
							User:      secondUser.Username,
							Group:     firstGroup.Name,
							RuleSummaries: []ruletree.Rule{
								{
									ID: 3,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98766, Files: 1, Size: 9},
										{Name: users.Username(2), MTime: 12346, Files: 2, Size: 12},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(1), MTime: 98766, Files: 3, Size: 21},
									},
								},
								{
									ID: 6,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98766, Files: 2, Size: 18},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(1), MTime: 98766, Files: 2, Size: 18},
									},
								},
							},
							Children: map[string]*ruletree.DirSummary{},
						},
					},
					Rules: map[uint64]*db.Rule{
						1: copyRule(rules[0]),
						2: copyRule(rules[1]),
						3: copyRule(rules[2]),
						4: copyRule(rules[3]),
						5: copyRule(rules[4]),
						6: copyRule(rules[5]),
					},
					Directories: map[string][]uint64{
						"/lustre/scratch123/humgen/a/b/":        {1, 2},
						"/lustre/scratch123/humgen/a/b/newdir/": {4, 5},
						"/lustre/scratch123/humgen/a/c/":        {3, 6},
					},
					BackupStatus: map[string]*ibackup.SetBackupActivity{
						"/lustre/scratch123/humgen/a/b/": {
							Name:      "plan::/lustre/scratch123/humgen/a/b/",
							Requester: "userA",
						},
						"/lustre/scratch123/humgen/a/b/newdir/": {
							Name:      "plan::/lustre/scratch123/humgen/a/b/newdir/",
							Requester: "userC",
						},
						"/lustre/scratch123/humgen/a/c/": {
							Name:      "plan::/lustre/scratch123/humgen/a/c/",
							Requester: "userB",
						},
						"userB:manualSetName": {
							Name:      "manualSetName",
							Requester: "userB",
						},
					},
					GroupBackupTypeTotals: map[string]map[int]*SizeCount{
						firstGroup.Name: {
							-1: {Count: 3, Size: 24},
							0:  {Count: 2, Size: 15},
							1:  {Count: 4, Size: 36},
							2:  {Count: 3, Size: 21},
						},
						secondGroup.Name: {
							-1: {Count: 1, Size: 8},
							0:  {Count: 1, Size: 8},
							1:  {Count: 1, Size: 8},
						},
					},
				}

				slices.Sort(gotSummary.Directories["/lustre/scratch123/humgen/a/b/"])

				gotSummary.BackupStatus["/lustre/scratch123/humgen/a/c/"].LastSuccess = time.Time{}

				So(gotSummary, ShouldResemble, expectedSummary)
			})
		})
	})
}

func awaitSummaryWithSuccessfulBackup(srv *Server, dir string, after time.Time) (summary, error) {
	const (
		pollEvery = 100 * time.Millisecond
		timeout   = 10 * time.Second
	)

	deadline := time.Now().Add(timeout)

	for {
		code, str := getResponse(srv.Summary, "/api/reports", nil)
		if code != http.StatusOK {
			if time.Now().After(deadline) {
				return summary{}, errTimeoutWaitingForSuccessfulBackup
			}

			time.Sleep(pollEvery)

			continue
		}

		var got summary

		err := json.NewDecoder(strings.NewReader(str)).Decode(&got)
		if err == nil {
			if status, ok := got.BackupStatus[dir]; ok && status != nil && status.LastSuccess.After(after) {
				return got, nil
			}
		}

		if time.Now().After(deadline) {
			return summary{}, errTimeoutWaitingForSuccessfulBackup
		}

		time.Sleep(pollEvery)
	}
}

// getSingleClientFromMultiClient returns the client from a MultiClient
// containing only one ibackup client.
func getSingleClientFromMultiClient(t *testing.T, client *ibackup.MultiClient) *server.Client {
	t.Helper()

	clientMap := *(*map[string]**atomic.Pointer[server.Client])(unsafe.Pointer(client))
	So(len(clientMap), ShouldEqual, 1)

	singleClient := *slices.Collect(maps.Values(clientMap))[0]

	return singleClient.Load()
}

// copyRule returns the rule without id's.
func copyRule(rule *db.Rule) *db.Rule {
	return &db.Rule{
		BackupType: rule.BackupType,
		Metadata:   rule.Metadata,
		Match:      rule.Match,
		Created:    rule.Created,
		Modified:   rule.Modified,
	}
}

func TestPopulateIbackupStatusWithFofnStatus(t *testing.T) {
	Convey("populateIbackupStatus adds both API and fofn status when both exist", t, func() {
		const (
			dir       = "/lustre/scratch/a/"
			claimedBy = "userA"
		)

		_, addr, certPath, dfn, err := internalibackup.NewTestIbackupServer(t)
		So(err, ShouldBeNil)
		Reset(func() {
			So(dfn(), ShouldBeNil)
		})

		tokenPath := filepath.Join(filepath.Dir(certPath), ".ibackup.token")
		baseDir := t.TempDir()

		mc, err := ibackup.New(ibackup.Config{
			Servers: map[string]ibackup.ServerDetails{
				"test": {
					Addr:    addr,
					Cert:    certPath,
					Token:   tokenPath,
					FofnDir: baseDir,
				},
			},
			PathToServer: map[string]ibackup.ServerTransformer{
				"^/lustre/scratch/a/": {ServerName: "test", Transformer: "prefix=/:/remote/"},
			},
		})
		So(err == nil || ibackup.IsOnlyConnectionErrors(err), ShouldBeTrue)
		Reset(mc.Stop)

		cc := ibackup.NewMultiCache(mc, 0)
		Reset(cc.Stop)

		planName := "plan::" + dir

		apiClient, err := ibackup.Connect(addr, certPath, "")
		So(err, ShouldBeNil)
		err = apiClient.AddOrUpdateSet(&set.Set{
			Name:        planName,
			Requester:   claimedBy,
			Transformer: "prefix=/:/remote/",
			Failed:      5,
		})
		So(err, ShouldBeNil)

		err = writeFofnStatus(baseDir, planName,
			"SUMMARY\tuploaded=1\treplaced=0\tunmodified=2\tmissing=0\t"+
				"failed=0\tfrozen=3\torphaned=0\twarning=0\thardlink=0\tnot_processed=0\n")
		So(err, ShouldBeNil)

		srv := &Server{config: configWithIbackupClients(mc, cc)}
		dirSummary := &summary{BackupStatus: make(map[string]*ibackup.SetBackupActivity)}

		srv.populateIbackupStatus(map[string]string{dir: claimedBy}, dirSummary)

		apiStatus, ok := dirSummary.BackupStatus[dir]
		So(ok, ShouldBeTrue)
		So(apiStatus, ShouldNotBeNil)
		So(apiStatus.Name, ShouldEqual, planName)
		So(apiStatus.Requester, ShouldEqual, claimedBy)
		So(apiStatus.Failures, ShouldEqual, 5)

		fofnStatus, ok := dirSummary.BackupStatus["fofn:"+dir]
		So(ok, ShouldBeTrue)
		So(fofnStatus, ShouldNotBeNil)
		So(fofnStatus.Name, ShouldEqual, planName)
		So(fofnStatus.Uploaded, ShouldEqual, 1)
		So(fofnStatus.Unmodified, ShouldEqual, 2)
		So(fofnStatus.Frozen, ShouldEqual, 3)
	})
}

func TestPopulateIbackupStatusWithoutFofnStatusFile(t *testing.T) {
	Convey("populateIbackupStatus only adds API entry when fofn status is missing", t, func() {
		const (
			dir       = "/lustre/scratch/a/"
			claimedBy = "userA"
		)

		_, addr, certPath, dfn, err := internalibackup.NewTestIbackupServer(t)
		So(err, ShouldBeNil)
		Reset(func() {
			So(dfn(), ShouldBeNil)
		})

		tokenPath := filepath.Join(filepath.Dir(certPath), ".ibackup.token")
		baseDir := t.TempDir()

		mc, err := ibackup.New(ibackup.Config{
			Servers: map[string]ibackup.ServerDetails{
				"test": {
					Addr:    addr,
					Cert:    certPath,
					Token:   tokenPath,
					FofnDir: baseDir,
				},
			},
			PathToServer: map[string]ibackup.ServerTransformer{
				"^/lustre/scratch/a/": {ServerName: "test", Transformer: "prefix=/:/remote/"},
			},
		})
		So(err == nil || ibackup.IsOnlyConnectionErrors(err), ShouldBeTrue)
		Reset(mc.Stop)

		cc := ibackup.NewMultiCache(mc, 0)
		Reset(cc.Stop)

		planName := "plan::" + dir

		apiClient, err := ibackup.Connect(addr, certPath, "")
		So(err, ShouldBeNil)
		err = apiClient.AddOrUpdateSet(&set.Set{
			Name:        planName,
			Requester:   claimedBy,
			Transformer: "prefix=/:/remote/",
			Failed:      2,
		})
		So(err, ShouldBeNil)

		srv := &Server{config: configWithIbackupClients(mc, cc)}
		dirSummary := &summary{BackupStatus: make(map[string]*ibackup.SetBackupActivity)}

		srv.populateIbackupStatus(map[string]string{dir: claimedBy}, dirSummary)

		_, ok := dirSummary.BackupStatus[dir]
		So(ok, ShouldBeTrue)
		_, ok = dirSummary.BackupStatus["fofn:"+dir]
		So(ok, ShouldBeFalse)
	})
}

func TestPopulateIbackupStatusWithFofnOnlyServer(t *testing.T) {
	Convey("populateIbackupStatus adds only fofn entry when only fofndir is configured", t, func() {
		const (
			dir       = "/lustre/scratch/a/"
			claimedBy = "userA"
		)

		baseDir := t.TempDir()
		planName := "plan::" + dir

		err := writeFofnStatus(baseDir, planName,
			"SUMMARY\tuploaded=4\treplaced=0\tunmodified=0\tmissing=0\t"+
				"failed=0\tfrozen=1\torphaned=0\twarning=0\thardlink=0\tnot_processed=0\n")
		So(err, ShouldBeNil)

		mc, err := ibackup.New(ibackup.Config{
			Servers: map[string]ibackup.ServerDetails{
				"fofn-only": {FofnDir: baseDir},
			},
			PathToServer: map[string]ibackup.ServerTransformer{
				"^/lustre/scratch/a/": {ServerName: "fofn-only", Transformer: "prefix=/:/remote/"},
			},
		})
		So(err, ShouldBeNil)
		Reset(mc.Stop)

		cc := ibackup.NewMultiCache(mc, 0)
		Reset(cc.Stop)

		srv := &Server{config: configWithIbackupClients(mc, cc)}
		dirSummary := &summary{BackupStatus: make(map[string]*ibackup.SetBackupActivity)}

		srv.populateIbackupStatus(map[string]string{dir: claimedBy}, dirSummary)

		_, ok := dirSummary.BackupStatus[dir]
		So(ok, ShouldBeFalse)

		fofnStatus, ok := dirSummary.BackupStatus["fofn:"+dir]
		So(ok, ShouldBeTrue)
		So(fofnStatus, ShouldNotBeNil)
		So(fofnStatus.Name, ShouldEqual, planName)
		So(fofnStatus.Uploaded, ShouldEqual, 4)
		So(fofnStatus.Frozen, ShouldEqual, 1)
	})
}

func writeFofnStatus(baseDir, setName, content string) error {
	statusDir := filepath.Join(baseDir, ibackup.SafeName(setName))

	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(statusDir, "status"), []byte(content), 0o600)
}

func configWithIbackupClients(mc *ibackup.MultiClient, cc *ibackup.MultiCache) *appconfig.Config {
	var c appconfig.Config

	prefix := (*struct {
		path                string
		mu                  sync.RWMutex
		ibackupClient       *ibackup.MultiClient
		ibackupCachedClient *ibackup.MultiCache
	})(unsafe.Pointer(&c))

	prefix.ibackupClient = mc
	prefix.ibackupCachedClient = cc

	return &c
}
