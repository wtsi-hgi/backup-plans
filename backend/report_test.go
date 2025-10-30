package backend

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ib "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/ruletree"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/ibackup/set"
	"vimagination.zapto.org/tree"
)

func TestReport(t *testing.T) {
	Convey("Given a test db, tree, roots and ibackup client, a backup can be made", t, func() {
		testDB, _ := plandb.PopulateExamplePlanDB(t)
		testTree := plandb.ExampleTree()
		path := filepath.Join(t.TempDir(), "testdb")
		file, err := os.Create(path)
		So(err, ShouldBeNil)
		err = tree.Serialise(file, testTree)
		So(err, ShouldBeNil)
		err = file.Close()
		So(err, ShouldBeNil)

		client := ib.NewClient(t)
		roots := []string{
			"/lustre/scratch123/humgen/a/c/",
			"/lustre/scratch123/humgen/a/b/",
		}

		server, err := New(testDB, func(_ *http.Request) string { return "test" }, roots, client)
		So(err, ShouldBeNil)
		err = server.AddTree(path)
		So(err, ShouldBeNil)

		exampleSet := &set.Set{
			Name:        "plan::/lustre/scratch123/humgen/a/c/",
			Requester:   "userB",
			Transformer: "humgen",
		}

		err = client.AddOrUpdateSet(exampleSet)
		So(err, ShouldBeNil)

		beforeTrigger := time.Now()

		err = client.TriggerDiscovery(exampleSet.ID(), false)
		So(err, ShouldBeNil)
		Convey("A summary can be retrieved", func() {
			code, str := getResponse(server.Summary, "/api/reports")
			So(code, ShouldEqual, http.StatusOK)

			var gotSummary summary

			err = json.NewDecoder(strings.NewReader(str)).Decode(&gotSummary)
			So(err, ShouldBeNil)

			rules := slices.Collect(testDB.ReadRules().Iter)

			Convey("Cointaining the correctly updated backup activity", func() {
				expectedSummary := summary{
					Summaries: map[string]*ruletree.DirSummary{
						"/lustre/scratch123/humgen/a/b/": {
							ClaimedBy: "userA",
							RuleSummaries: []ruletree.Rule{
								{
									ID: 0,
									Users: ruletree.RuleStats{
										{Name: users.Username(1), MTime: 98767, Files: 1, Size: 8},
										{Name: users.Username(2), MTime: 12346, Files: 1, Size: 6},
									},
									Groups: ruletree.RuleStats{
										{Name: users.Group(1), MTime: 12346, Files: 1, Size: 6},
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
							},
							Children: map[string]*ruletree.DirSummary{
								"testdir/": {
									ClaimedBy: "",
									RuleSummaries: []ruletree.Rule{
										{
											ID: 0,
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
							RuleSummaries: []ruletree.Rule{
								{
									ID: 3,
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
					Rules: map[uint64]*db.Rule{
						0: nil,
						1: copyRule(rules[0]),
						2: copyRule(rules[1]),
						3: copyRule(rules[2]),
					},
					Directories: map[string][]uint64{
						"/lustre/scratch123/humgen/a/b/": {1, 2},
						"/lustre/scratch123/humgen/a/c/": {3},
					},
					BackupStatus: map[string]*ibackup.SetBackupActivity{
						"/lustre/scratch123/humgen/a/b/": nil,
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
		ReviewDate: rule.ReviewDate,
		RemoveDate: rule.RemoveDate,
		Match:      rule.Match,
		Frequency:  rule.Frequency,
		Created:    rule.Created,
		Modified:   rule.Modified,
	}
}
