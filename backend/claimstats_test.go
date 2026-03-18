/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Sky Haines <sh55@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package backend

import (
	"encoding/json"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"vimagination.zapto.org/tree"
)

func TestClaimStats(t *testing.T) {
	Convey("With a configured backend", t, func() {
		var u userHandler

		firstGroup, err := user.LookupGroupId("1")
		So(err, ShouldBeNil)

		testDB, _ := plandb.PopulateExamplePlanDB(t)
		tr := plandb.ExampleTree()

		treeFile := filepath.Join(t.TempDir(), "tree.db")
		f, err := os.Create(treeFile)
		So(err, ShouldBeNil)

		So(tree.Serialise(f, tr), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		s, err := New(testDB, u.getUser, config.NewConfig(t, nil, nil, nil, 0))
		So(err, ShouldBeNil)

		So(s.AddTree(treeFile), ShouldBeNil)

		Convey("Claimstats should return all claimed directories, filtered by user, group or bom", func() {
			u = userA

			code, resp := getResponse(s.ClaimStats, "/api/claimstats?user=userA", nil)
			So(code, ShouldEqual, http.StatusOK)

			var claimstatsA []DirStats

			err = json.NewDecoder(strings.NewReader(resp)).Decode(&claimstatsA)
			So(err, ShouldBeNil)

			rules := slices.Collect(testDB.ReadRules().Iter)

			So(claimstatsA, ShouldResemble, []DirStats{
				{
					Path:      "/lustre/scratch123/humgen/a/b/",
					ClaimedBy: "userA",
					Group:     firstGroup.Name,
					BackupStatus: []*ibackup.SetBackupActivity{
						{
							Name:      "plan::/lustre/scratch123/humgen/a/b/",
							Requester: userA,
						},
					},
					RuleStats: []ruleStats{
						{
							Rule: nil,
							SizeCount: SizeCount{
								Size:  14,
								Count: 2,
							},
						},
						{
							Rule: copyRule(rules[0]),
							SizeCount: SizeCount{
								Size:  17,
								Count: 2,
							},
						},
						{
							Rule: copyRule(rules[1]),
							SizeCount: SizeCount{
								Size:  8,
								Count: 1,
							},
						},
					},
				},
			})

			u = userB
			code, resp = getResponse(s.ClaimStats, "/api/claimstats?groupbom="+firstGroup.Name, nil)

			So(code, ShouldEqual, http.StatusOK)

			var claimstatsB []DirStats

			err = json.NewDecoder(strings.NewReader(resp)).Decode(&claimstatsB)
			So(err, ShouldBeNil)

			rules = slices.Collect(testDB.ReadRules().Iter)

			So(claimstatsB, ShouldResemble, []DirStats{
				{
					Path:      "/lustre/scratch123/humgen/a/b/",
					ClaimedBy: "userA",
					Group:     firstGroup.Name,
					BackupStatus: []*ibackup.SetBackupActivity{
						{
							Name:      "plan::/lustre/scratch123/humgen/a/b/",
							Requester: userA,
						},
					},
					RuleStats: []ruleStats{
						{
							Rule: nil,
							SizeCount: SizeCount{
								Size:  14,
								Count: 2,
							},
						},
						{
							Rule: copyRule(rules[0]),
							SizeCount: SizeCount{
								Size:  17,
								Count: 2,
							},
						},
						{
							Rule: copyRule(rules[1]),
							SizeCount: SizeCount{
								Size:  8,
								Count: 1,
							},
						},
					},
				},
				{
					Path:      "/lustre/scratch123/humgen/a/c/",
					ClaimedBy: userB,
					Group:     firstGroup.Name,
					BackupStatus: []*ibackup.SetBackupActivity{
						{
							Name:      "manualSetName",
							Requester: userB,
						},
					},
					RuleStats: []ruleStats{
						{
							Rule: copyRule(rules[2]),
							SizeCount: SizeCount{
								Size:  6,
								Count: 1,
							},
						},
					},
				},
			})
		})
	})
}
