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
	"path/filepath"
	"slices"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/config"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"vimagination.zapto.org/tree"
)

func TestClaimStats(t *testing.T) {
	Convey("With a configured backend", t, func() {
		var u userHandler

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

		Convey("Claimstats should return all claimed directories per user", func() {
			code, resp := getResponse(s.ClaimStats, "/api/claimstats", nil)
			So(code, ShouldEqual, http.StatusOK)

			var claimstats map[string]map[string][]RuleStats

			err = json.NewDecoder(strings.NewReader(resp)).Decode(&claimstats)
			So(err, ShouldBeNil)

			rules := slices.Collect(testDB.ReadRules().Iter)

			So(claimstats, ShouldResemble, map[string]map[string][]RuleStats{
				"userA": {
					"/lustre/scratch123/humgen/a/b/": {
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
				"userB": {
					"/lustre/scratch123/humgen/a/c/": {
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
