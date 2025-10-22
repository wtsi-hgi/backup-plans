/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
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

package ruletree

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/users"
	"vimagination.zapto.org/tree"
)

func TestRuletree(t *testing.T) {
	Convey("Given Tree DBs and rules", t, func() {
		treeDBA := directories.NewRoot("/some/path/", time.Now().Unix())
		directories.AddFile(&treeDBA.Directory, "MyDir/a.txt", 1, 2, 3, 4)
		directories.AddFile(&treeDBA.Directory, "MyDir/b.csv", 1, 2, 5, 6)
		directories.AddFile(&treeDBA.Directory, "YourDir/c.tsv", 21, 22, 15, 16)
		directories.AddFile(&treeDBA.Directory, "OtherDir/a.file", 1, 22, 25, 26)
		directories.AddFile(&treeDBA.Directory, "OtherDir/b.file", 1, 2, 35, 36)

		treeDBB := directories.NewRoot("/some/other/path/", time.Now().Unix())

		treeDBPathA := filepath.Join(t.TempDir(), "a.db")
		treeDBPathB := filepath.Join(t.TempDir(), "b.db")

		f, err := os.Create(treeDBPathA)
		So(err, ShouldBeNil)
		So(tree.Serialise(f, treeDBA), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		f, err = os.Create(treeDBPathB)
		So(err, ShouldBeNil)
		So(tree.Serialise(f, treeDBB), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		tdb := testdb.CreateTestDatabase(t)

		dirs := []db.Directory{
			{Path: "/some/path/MyDir/"},
			{Path: "/some/path/YourDir/"},
		}
		rules := []db.Rule{
			{Match: "*"},
			{Match: "*.txt"},
			{Match: "*"},
		}

		So(tdb.CreateDirectory(&dirs[0]), ShouldBeNil)
		So(tdb.CreateDirectory(&dirs[1]), ShouldBeNil)

		So(tdb.CreateDirectoryRule(&dirs[0], &rules[0]), ShouldBeNil)
		So(tdb.CreateDirectoryRule(&dirs[0], &rules[1]), ShouldBeNil)
		So(tdb.CreateDirectoryRule(&dirs[1], &rules[2]), ShouldBeNil)

		Convey("You can create a RuleTree", func() {
			root, err := NewRoot([]DirRule{
				{Directory: &dirs[0], Rule: &rules[0]},
				{Directory: &dirs[0], Rule: &rules[1]},
				{Directory: &dirs[1], Rule: &rules[2]},
			})
			So(err, ShouldBeNil)

			So(root.AddTree(treeDBPathA), ShouldBeNil)
			So(root.AddTree(treeDBPathB), ShouldBeNil)

			ruleExpectations := []Rule{
				{
					ID: 0,
					Users: RuleStats{
						{
							id:    1,
							Name:  users.Username(1),
							MTime: 36,
							Files: 2,
							Size:  60,
						},
					},
					Groups: RuleStats{
						{
							id:    2,
							Name:  users.Group(2),
							MTime: 36,
							Files: 1,
							Size:  35,
						},
						{
							id:    22,
							Name:  users.Group(22),
							MTime: 26,
							Files: 1,
							Size:  25,
						},
					},
				},
				{
					ID: 1,
					Users: RuleStats{
						{
							id:    1,
							Name:  users.Username(1),
							MTime: 6,
							Files: 1,
							Size:  5,
						},
					},
					Groups: RuleStats{
						{
							id:    2,
							Name:  users.Group(2),
							MTime: 6,
							Files: 1,
							Size:  5,
						},
					},
				},
				{
					ID: 2,
					Users: RuleStats{
						{
							id:    1,
							Name:  users.Username(1),
							MTime: 4,
							Files: 1,
							Size:  3,
						},
					},
					Groups: RuleStats{
						{
							id:    2,
							Name:  users.Group(2),
							MTime: 4,
							Files: 1,
							Size:  3,
						},
					},
				},
				{
					ID: 3,
					Users: RuleStats{
						{
							id:    21,
							Name:  users.Username(21),
							MTime: 16,
							Files: 1,
							Size:  15,
						},
					},
					Groups: RuleStats{
						{
							id:    22,
							Name:  users.Group(22),
							MTime: 16,
							Files: 1,
							Size:  15,
						},
					},
				},
			}

			s, err := root.Summary("")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &DirSummary{
				RuleSummaries: ruleExpectations,
				Children: map[string]*DirSummary{
					"some/": {
						RuleSummaries: ruleExpectations,
					},
				},
			})

			s, err = root.Summary("some/")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &DirSummary{
				RuleSummaries: ruleExpectations,
				Children: map[string]*DirSummary{
					"other/": {
						RuleSummaries: []Rule{},
					},
					"path/": {
						RuleSummaries: ruleExpectations,
					},
				},
			})

			s, err = root.Summary("some/path/")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &DirSummary{
				RuleSummaries: ruleExpectations,
				Children: map[string]*DirSummary{
					"MyDir/": {
						RuleSummaries: []Rule{ruleExpectations[1], ruleExpectations[2]},
						Children:      map[string]*DirSummary{},
					},
					"YourDir/": {
						RuleSummaries: []Rule{ruleExpectations[3]},
						Children:      map[string]*DirSummary{},
					},
					"OtherDir/": {
						RuleSummaries: []Rule{ruleExpectations[0]},
						Children:      map[string]*DirSummary{},
					},
				},
			})

			s, err = root.Summary("some/path/MyDir/")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &DirSummary{
				RuleSummaries: []Rule{ruleExpectations[1], ruleExpectations[2]},
				Children:      map[string]*DirSummary{},
			})
		})
	})
}
