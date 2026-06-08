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
	"path/filepath"
	"slices"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/backuptree"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/memtree"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/rules"
)

func TestRoot(t *testing.T) {
	Convey("Given a rules Database and a directory tree", t, func() {
		treeDBA := directories.NewRoot("/some/path/", time.Now().Unix())
		directories.AddFile(&treeDBA.Directory, "MyDir/a.txt", 0, 0, 3, 4)
		directories.AddFile(&treeDBA.Directory, "MyDir/b.csv", 0, 0, 5, 6)

		treeDBPathA := filepath.Join(t.TempDir(), "a.db")

		So(memtree.TreeToFile(treeDBA, treeDBPathA), ShouldBeNil)

		root := newEmptyRoot(t)

		_, err := root.AddTree(treeDBPathA)
		So(err, ShouldBeNil)

		Convey("You can claim, transfer, and revoke directories", func() {
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

			So(root.RevokeDirectory("/some/path/MyDir/"), ShouldBeNil)

			claimed = slices.Collect(root.rules.Dirs())
			So(len(claimed), ShouldEqual, 0)
		})

		Convey("Claiming a directory before the tree is added still creates the cache", func() {
			So(root.ClaimDirectory("/some/other/path/YourDir/", "me"), ShouldBeNil)

			treeDBB := directories.NewRoot("/some/other/path/", time.Now().Unix())
			directories.AddFile(&treeDBB.Directory, "YourDir/a.txt", 0, 0, 3, 4)
			directories.AddFile(&treeDBB.Directory, "YourDir/b.csv", 0, 0, 5, 6)

			treeDBPathB := filepath.Join(t.TempDir(), "b.db")

			So(memtree.TreeToFile(treeDBB, treeDBPathB), ShouldBeNil)

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

				_, err := newRoot.AddTree(treeDBPathB)
				So(err, ShouldBeNil)

				So(newRoot.claimed, ShouldResemble, expectation)
			})
		})
	})
}

func TestClaims(t *testing.T) {
	Convey("Given a rules Database and a directory tree", t, func() {
		treeDBA := directories.NewRoot("/some/path/", time.Now().Unix())
		directories.AddFile(&treeDBA.Directory, "MyDir/a.txt", 0, 0, 3, 4)
		directories.AddFile(&treeDBA.Directory, "MyDir/Child/b.csv", 0, 0, 5, 6)
		directories.AddFile(&treeDBA.Directory, "Root/OtherDir/Child/c.csv", 0, 0, 5, 6)

		treeDBPathA := filepath.Join(t.TempDir(), "a.db")

		So(memtree.TreeToFile(treeDBA, treeDBPathA), ShouldBeNil)

		root := newEmptyRoot(t)

		_, err := root.AddTree(treeDBPathA)
		So(err, ShouldBeNil)

		Convey("You can claim, pass, and revoke directories which updates the claimed caches", func() {
			So(root.ClaimDirectory("/some/path/MyDir/", "me"), ShouldBeNil)
			So(root.claimed["/some/path/MyDir/"].ClaimedBy, ShouldEqual, "me")
			So(root.claimed["/some/path/MyDir/"].Children["Child/"].ClaimedBy, ShouldEqual, "")

			So(root.ClaimDirectory("/some/path/MyDir/Child/", "you"), ShouldBeNil)
			So(root.claimed["/some/path/MyDir/Child/"].ClaimedBy, ShouldEqual, "you")
			So(root.claimed["/some/path/MyDir/"].Children["Child/"].ClaimedBy, ShouldEqual, "you")

			So(root.PassDirectory("/some/path/MyDir/Child/", "other"), ShouldBeNil)
			So(root.claimed["/some/path/MyDir/Child/"].ClaimedBy, ShouldEqual, "other")
			So(root.claimed["/some/path/MyDir/"].Children["Child/"].ClaimedBy, ShouldEqual, "other")

			So(root.RevokeDirectory("/some/path/MyDir/Child/"), ShouldBeNil)
			So(root.claimed["/some/path/MyDir/Child/"], ShouldBeNil)
			So(root.claimed["/some/path/MyDir/"].Children["Child/"].ClaimedBy, ShouldEqual, "")

			So(root.ClaimDirectory("/some/path/NoDir/", "me"), ShouldBeNil)

			dir, ok := root.claimed["/some/path/NoDir/"]
			So(ok, ShouldBeTrue)
			So(dir, ShouldBeNil)
		})

		Convey("Cached directories get updated when the claimed status changes", func() {
			root.CacheSummaries("/some/path/Root/")
			So(root.cached["/some/path/Root/"], ShouldNotBeNil)
			So(root.cached["/some/path/Root/"].ClaimedBy, ShouldEqual, "")
			So(root.cached["/some/path/Root/"].Children["OtherDir/"].ClaimedBy, ShouldEqual, "")

			So(root.ClaimDirectory("/some/path/Root/OtherDir/", "me"), ShouldBeNil)
			So(root.cached["/some/path/Root/"].ClaimedBy, ShouldEqual, "")
			So(root.cached["/some/path/Root/"].Children["OtherDir/"].ClaimedBy, ShouldEqual, "me")

			So(root.PassDirectory("/some/path/Root/OtherDir/", "you"), ShouldBeNil)
			So(root.cached["/some/path/Root/"].Children["OtherDir/"].ClaimedBy, ShouldEqual, "you")

			So(root.RevokeDirectory("/some/path/Root/OtherDir/"), ShouldBeNil)
			So(root.cached["/some/path/Root/"].Children["OtherDir/"].ClaimedBy, ShouldEqual, "")

			So(root.ClaimDirectory("/some/path/Root/", "me"), ShouldBeNil)
			So(root.cached["/some/path/Root/"].ClaimedBy, ShouldEqual, "me")

			So(root.PassDirectory("/some/path/Root/", "you"), ShouldBeNil)
			So(root.cached["/some/path/Root/"].ClaimedBy, ShouldEqual, "you")

			So(root.RevokeDirectory("/some/path/Root/"), ShouldBeNil)
			So(root.cached["/some/path/Root/"].ClaimedBy, ShouldEqual, "")
		})
	})
}

func TestBackups(t *testing.T) {
	Convey("Given a backup tree", t, func() {
		root := newEmptyRoot(t)
		bt := filepath.Join(t.TempDir(), "backups.db")

		So(memtree.TreeToFile(backuptree.Generate(map[string]map[string]uint64{
			"/some/path/MyDir/": {
				"/a.txt": 1,
				"/b.csv": 2,
			},
			"/some/path/MyDir/more/": {
				"/another.txt": 5,
			},
			"/some/path/YourDir/": {
				"/a.txt":     999,
				"/dir/b.txt": 1234,
			},
		}), bt), ShouldBeNil)
		So(root.SetBackupTree(bt), ShouldBeNil)

		Convey("You can list the backed-up files for a directory", func() {
			var paths []string

			collect := func(path string, _ BackupStats) error {
				paths = append(paths, path)

				return nil
			}

			So(root.BackedUpFiles("/some/path/MyDir/", false).ForEach(collect), ShouldBeNil)
			So(paths, ShouldResemble, []string{
				"/some/path/MyDir/a.txt",
				"/some/path/MyDir/b.csv",
			})

			paths = paths[:0]

			So(root.BackedUpFiles("/some/path/YourDir/", false).ForEach(collect), ShouldBeNil)
			So(paths, ShouldResemble, []string{
				"/some/path/YourDir/a.txt",
				"/some/path/YourDir/dir/b.txt",
			})

			paths = paths[:0]

			So(root.BackedUpFiles("/some/path/OtherDir/", false).ForEach(collect), ShouldBeNil)
			So(paths, ShouldBeEmpty)
		})

		Convey("You can list the files in a backup set", func() {
			var files []string

			collectFiles := func(name string, _ BackupStats) error {
				files = append(files, name)

				return nil
			}

			So(root.BackedUpFiles("/some/path/MyDir/", false).ForEach(collectFiles), ShouldBeNil)
			So(files, ShouldResemble, []string{
				"/some/path/MyDir/a.txt",
				"/some/path/MyDir/b.csv",
			})

			files = files[:0]

			So(root.BackedUpFiles("/some/path/MyDir/", true).ForEach(collectFiles), ShouldBeNil)
			So(files, ShouldResemble, []string{
				"/some/path/MyDir/a.txt",
				"/some/path/MyDir/b.csv",
				"/some/path/MyDir/more/another.txt",
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
