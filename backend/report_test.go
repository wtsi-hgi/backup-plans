package backend

import (
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"vimagination.zapto.org/tree"
)

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

		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		roots := []string{
			"/lustre/scratch123/humgen/a/[bc]/",
		}

		srv, err := New(testDB, func(_ *http.Request) string { return "test" }, config.NewConfig(t, nil, nil, roots, 0))
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
			code, str := getResponse(srv.Summary, "/api/reports", nil)
			So(code, ShouldEqual, http.StatusOK)

			var gotSummary summary

			err = json.NewDecoder(strings.NewReader(str)).Decode(&gotSummary)
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
						"/lustre/scratch123/humgen/a/c/": {
							Name:      "plan::/lustre/scratch123/humgen/a/c/",
							Requester: "userB",
						},
					},
				}

				slices.Sort(gotSummary.Directories["/lustre/scratch123/humgen/a/b/"])

				So(gotSummary.BackupStatus["/lustre/scratch123/humgen/a/c/"].LastSuccess, ShouldHappenAfter, beforeTrigger)

				gotSummary.BackupStatus["/lustre/scratch123/humgen/a/c/"].LastSuccess = time.Time{}

				So(gotSummary, ShouldResemble, expectedSummary)
			})
		})
	})
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

// getSingleClientFromMultiClient returns the client from a MultiClient
// containing only one ibackup client.
func getSingleClientFromMultiClient(t *testing.T, client *ibackup.MultiClient) *server.Client {
	t.Helper()

	clientMap := *(*map[string]**atomic.Pointer[server.Client])(unsafe.Pointer(client))
	So(len(clientMap), ShouldEqual, 1)

	singleClient := *slices.Collect(maps.Values(clientMap))[0]

	return singleClient.Load()
}
