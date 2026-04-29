/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
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
	"slices"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/rules"
	"vimagination.zapto.org/tree"
)

func TestRoot(t *testing.T) {
	Convey("Given a rules Database and a directory tree", t, func() {
		treeDBA := directories.NewRoot("/some/path/", time.Now().Unix())
		directories.AddFile(&treeDBA.Directory, "MyDir/a.txt", 0, 0, 3, 4)
		directories.AddFile(&treeDBA.Directory, "MyDir/b.csv", 0, 0, 5, 6)

		treeDBPathA := filepath.Join(t.TempDir(), "a.db")

		f, err := os.Create(treeDBPathA)
		So(err, ShouldBeNil)
		So(tree.Serialise(f, treeDBA), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		root := newEmptyRoot(t)

		_, err = root.AddTree(treeDBPathA)
		So(err, ShouldBeNil)

		Convey("You can claim, tranfer, and revoke directories", func() {
			So(root.ClaimDirectory("/some/path/MyDir/", "me"), ShouldBeNil)

			claimed := slices.Collect(root.rules.Dirs())
			So(len(claimed), ShouldEqual, 1)
			So(claimed[0].Path, ShouldEqual, "/some/path/MyDir/")
			So(claimed[0].ClaimedBy, ShouldEqual, "me")

			So(root.claimed, ShouldResemble, map[string]*DirSummary{
				"/some/path/MyDir/": {
					User:      "root",
					Group:     "root",
					ClaimedBy: "me",
					RuleSummaries: []Rule{
						{
							Users: RuleStats{
								{
									Name:  "root",
									MTime: 6,
									Files: 2,
									Size:  8,
								},
							},
							Groups: RuleStats{
								{
									Name:  "root",
									MTime: 6,
									Files: 2,
									Size:  8,
								},
							},
						},
					},
					Children: map[string]*DirSummary{},
					LastMod:  6,
				},
			})

			So(root.ClaimDirectory("/some/path/MyDir/", "me"), ShouldEqual, rules.ErrDirectoryClaimed)
			So(root.PassDirectory("/some/path/MyDir/", "you"), ShouldBeNil)

			claimed = slices.Collect(root.rules.Dirs())
			So(len(claimed), ShouldEqual, 1)
			So(claimed[0].Path, ShouldEqual, "/some/path/MyDir/")
			So(claimed[0].ClaimedBy, ShouldEqual, "you")

			So(root.RevokeDirectory("/some/path/MyDir/", "you"), ShouldBeNil)

			claimed = slices.Collect(root.rules.Dirs())
			So(len(claimed), ShouldEqual, 0)
		})

		Convey("Claiming a directory before the tree is added still creates the cache", func() {
			So(root.ClaimDirectory("/some/other/path/YourDir/", "me"), ShouldBeNil)

			treeDBB := directories.NewRoot("/some/other/path/", time.Now().Unix())
			directories.AddFile(&treeDBB.Directory, "YourDir/a.txt", 0, 0, 3, 4)
			directories.AddFile(&treeDBB.Directory, "YourDir/b.csv", 0, 0, 5, 6)

			treeDBPathB := filepath.Join(t.TempDir(), "b.db")

			f, err := os.Create(treeDBPathB)
			So(err, ShouldBeNil)
			So(tree.Serialise(f, treeDBB), ShouldBeNil)
			So(f.Close(), ShouldBeNil)

			_, err = root.AddTree(treeDBPathB)
			So(err, ShouldBeNil)

			expectation := map[string]*DirSummary{
				"/some/other/path/YourDir/": {
					User:      "root",
					Group:     "root",
					ClaimedBy: "me",
					RuleSummaries: []Rule{
						{
							Users: RuleStats{
								{
									Name:  "root",
									MTime: 6,
									Files: 2,
									Size:  8,
								},
							},
							Groups: RuleStats{
								{
									Name:  "root",
									MTime: 6,
									Files: 2,
									Size:  8,
								},
							},
						},
					},
					Children: map[string]*DirSummary{},
					LastMod:  6,
				},
			}

			So(root.claimed, ShouldResemble, expectation)

			Convey("Rebuilding the root from the DB creates the cache", func() {
				newRoot := NewRoot(root.rules)

				newRoot.AddTree(treeDBPathB)

				So(newRoot.claimed, ShouldResemble, expectation)
			})
		})
	})
}

func newEmptyRoot(t *testing.T) *RootDir {
	t.Helper()

	return newRoot(t, testdb.CreateTestDatabase(t))
}

func newRoot(t *testing.T, db *db.DB) *RootDir {
	t.Helper()

	rdb, err := rules.New(db)
	So(err, ShouldBeNil)

	return NewRoot(rdb)
}
