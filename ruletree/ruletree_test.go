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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"github.com/wtsi-hgi/backup-plans/internal/plandb"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
	"github.com/wtsi-hgi/backup-plans/users"
	"vimagination.zapto.org/tree"
)

func TestRuletree(t *testing.T) {
	Convey("Given Tree DBs and a database", t, func() {
		tdb := testdb.CreateTestDatabase(t)

		Convey("You can create a RuleTree", func() {
			treeDBA := directories.NewRoot("/some/path/", time.Now().Unix())
			directories.AddFile(&treeDBA.Directory, "MyDir/a.txt", 1, 2, 3, 4)
			directories.AddFile(&treeDBA.Directory, "MyDir/b.csv", 1, 2, 5, 6)
			directories.AddFile(&treeDBA.Directory, "YourDir/c.tsv", 21, 22, 15, 16)
			directories.AddFile(&treeDBA.Directory, "OtherDir/a.file", 1, 22, 25, 26)
			directories.AddFile(&treeDBA.Directory, "OtherDir/b.file", 1, 2, 35, 36)

			treeDBB := directories.NewRoot("/some/other/path/", time.Now().Unix())

			treeDBPathA := createTree(t, treeDBA)
			treeDBPathB := createTree(t, treeDBB)

			rules := genRules(t, tdb, map[string][]string{
				"/some/path/MyDir/":   {"*", "*.txt"},
				"/some/path/YourDir/": {"*"},
			})

			root, err := NewRoot(rules)
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
					ID: uint64(rules[0].Rule.ID()), //nolint:gosec
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
					ID: uint64(rules[1].Rule.ID()), //nolint:gosec
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
					ID: uint64(rules[2].Rule.ID()), //nolint:gosec
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

			s, err := root.Summary("/")
			So(err, ShouldBeNil)

			So(s, ShouldResemble, &DirSummary{
				RuleSummaries: ruleExpectations,
				Children: map[string]*DirSummary{
					"some/": {
						RuleSummaries: ruleExpectations,
					},
				},
			})

			s, err = root.Summary("/some/")
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

			s, err = root.Summary("/some/path/")
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

			s, err = root.Summary("/some/path/MyDir/")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &DirSummary{
				RuleSummaries: []Rule{ruleExpectations[1], ruleExpectations[2]},
				Children:      map[string]*DirSummary{},
			})
		})

		Convey("You can add and remove rules with basic wildcard matches", func() {
			root, err := NewRoot(nil)
			So(err, ShouldBeNil)

			treeDB := buildTreeDB(t, []string{
				"/path/wildcard/file.txt",
				"/path/wildcard/sub/file.txt",
				"/path/wildcard/sub2/file.txt",
				"/path/wildcard/sub2/sub3/file.txt",
			})

			treeDBPath := createTree(t, treeDB)

			So(root.AddTree(treeDBPath), ShouldBeNil)

			wildcardRuleID := createRule(t, tdb, root, "/path/wildcard/", "*")

			So(ruleIDCount(t, root, "/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 4})
			So(ruleIDCount(t, root, "/path/wildcard/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 4})
			So(ruleIDCount(t, root, "/path/wildcard/sub/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 1})

			createRule(t, tdb, root, "/path/wildcard/", "file.txt.tsv")

			So(ruleIDCount(t, root, "/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 4})
			So(ruleIDCount(t, root, "/path/wildcard/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 4})
			So(ruleIDCount(t, root, "/path/wildcard/sub/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 1})

			wildcardRuleIDC := createRule(t, tdb, root, "/path/wildcard/", "file.txt")

			So(ruleIDCount(t, root, "/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 3, wildcardRuleIDC: 1})
			So(ruleIDCount(t, root, "/path/wildcard/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 3, wildcardRuleIDC: 1})
			So(ruleIDCount(t, root, "/path/wildcard/sub/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 1})

			wildcardRuleIDD := createRule(t, tdb, root, "/path/wildcard/sub2/", "*")

			So(ruleIDCount(t, root, "/"), ShouldResemble, map[uint64]uint64{
				wildcardRuleID: 1, wildcardRuleIDC: 1, wildcardRuleIDD: 2,
			})
			So(ruleIDCount(t, root, "/path/wildcard/"), ShouldResemble, map[uint64]uint64{
				wildcardRuleID: 1, wildcardRuleIDC: 1, wildcardRuleIDD: 2,
			})
			So(ruleIDCount(t, root, "/path/wildcard/sub/"), ShouldResemble, map[uint64]uint64{wildcardRuleID: 1})
			So(ruleIDCount(t, root, "/path/wildcard/sub2/"), ShouldResemble, map[uint64]uint64{wildcardRuleIDD: 2})
			So(ruleIDCount(t, root, "/path/wildcard/sub2/sub3/"), ShouldResemble, map[uint64]uint64{wildcardRuleIDD: 1})

			RemoveRule(t, tdb, root, "/path/wildcard/", "*")

			So(ruleIDCount(t, root, "/"), ShouldResemble, map[uint64]uint64{0: 1, wildcardRuleIDC: 1, wildcardRuleIDD: 2})
			So(ruleIDCount(t, root, "/path/wildcard/"), ShouldResemble, map[uint64]uint64{
				0: 1, wildcardRuleIDC: 1, wildcardRuleIDD: 2,
			})
			So(ruleIDCount(t, root, "/path/wildcard/sub/"), ShouldResemble, map[uint64]uint64{0: 1})
			So(ruleIDCount(t, root, "/path/wildcard/sub2/"), ShouldResemble, map[uint64]uint64{wildcardRuleIDD: 2})
			So(ruleIDCount(t, root, "/path/wildcard/sub2/sub3/"), ShouldResemble, map[uint64]uint64{wildcardRuleIDD: 1})
		})

		Convey("You can add and remove rules with complex matches", func() {
			root, err := NewRoot(nil)
			So(err, ShouldBeNil)

			treeDB := buildTreeDB(t, []string{
				"/path/temp/complex1a.txt",
				"/path/temp/complex1b.txt",
				"/path/temp/complex2a.log",
				"/path/temp/complex2b.log",
				"/path/temp/test/complex4a.txt",
				"/path/temp/complexa/test.cram",
			})

			treeDBPath := createTree(t, treeDB)
			So(root.AddTree(treeDBPath), ShouldBeNil)

			createRule(t, tdb, root, "/path/temp/", "complex*a")
			So(ruleIDCount(t, root, "/"), ShouldResemble, map[uint64]uint64{0: 6})
			So(ruleIDCount(t, root, "/path/temp/"), ShouldResemble, map[uint64]uint64{0: 6})

			complexRuleIDB := createRule(t, tdb, root, "/path/temp/", "complex*a*")
			So(ruleIDCount(t, root, "/path/temp/"), ShouldResemble, map[uint64]uint64{0: 3, complexRuleIDB: 3})

			complexRuleIDC := createRule(t, tdb, root, "/path/temp/", "*test*")
			So(ruleIDCount(t, root, "/path/temp/"), ShouldResemble, map[uint64]uint64{
				0: 2, complexRuleIDB: 3, complexRuleIDC: 1,
			})

			RemoveRule(t, tdb, root, "/path/temp/", "complex*a*")
			So(ruleIDCount(t, root, "/path/temp/"), ShouldResemble, map[uint64]uint64{0: 4, complexRuleIDC: 2})
		})

		Convey("You can add and remove rules with complex matches and simple wildcards", func() {
			root, err := NewRoot(nil)
			So(err, ShouldBeNil)

			treeDB := buildTreeDB(t, []string{
				"/path/dir/file1.txt",
				"/path/dir/file2.log",
				"/path/dir/subdir/file3.txt",
				"/path/dir/subdir/file4.log",
			})

			treeDBPath := createTree(t, treeDB)
			So(root.AddTree(treeDBPath), ShouldBeNil)

			r1 := createRule(t, tdb, root, "/path/dir/", "f*")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{0: 2, r1: 2})

			r2 := createRule(t, tdb, root, "/path/dir/", "*")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{r1: 2, r2: 2})

			RemoveRule(t, tdb, root, "/path/dir/", "f*")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{r2: 4})
		})

		Convey("You can remove a child rule and numbers update correctly", func() {
			root, err := NewRoot(nil)
			So(err, ShouldBeNil)

			treeDB := buildTreeDB(t, []string{
				"/path/dir/file1.txt",
				"/path/dir/file2.log",
				"/path/dir/subdir/file3.txt",
				"/path/dir/subdir/file4.log",
			})

			treeDBPath := createTree(t, treeDB)
			So(root.AddTree(treeDBPath), ShouldBeNil)

			r1 := createRule(t, tdb, root, "/path/dir/", "*.txt")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{0: 2, r1: 2})

			r2 := createRule(t, tdb, root, "/path/dir/subdir/", "*")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{0: 1, r1: 1, r2: 2})

			RemoveRule(t, tdb, root, "/path/dir/subdir/", "*")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{0: 2, r1: 2})
		})

		Convey("Override wildcard rules are correctly handled in a summary", func() {
			root, err := NewRoot(nil)
			So(err, ShouldBeNil)

			treeDB := buildTreeDB(t, []string{
				"/path/dir/a/file1.txt",
				"/path/dir/a/b/file2.txt",
			})

			treeDBPath := createTree(t, treeDB)
			So(root.AddTree(treeDBPath), ShouldBeNil)

			r1 := createRule(t, tdb, root, "/path/dir/", "b/*", true)
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{0: 1, r1: 1})
			So(ruleIDCount(t, root, "/path/dir/a/"), ShouldResemble, map[uint64]uint64{0: 1, r1: 1})
			So(ruleIDCount(t, root, "/path/dir/a/b/"), ShouldResemble, map[uint64]uint64{r1: 1})

			r2 := createRule(t, tdb, root, "/path/dir/a/", "*")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{r1: 1, r2: 1})
			So(ruleIDCount(t, root, "/path/dir/a/"), ShouldResemble, map[uint64]uint64{r1: 1, r2: 1})
			So(ruleIDCount(t, root, "/path/dir/a/b/"), ShouldResemble, map[uint64]uint64{r1: 1})
		})

		Convey("Override wildcard rules with two or more levels of depth are correctly handled in a summary", func() {
			root, err := NewRoot(nil)
			So(err, ShouldBeNil)

			treeDB := buildTreeDB(t, []string{
				"/path/dir/a/file1.txt",
				"/path/dir/a/b/file2.txt",
				"/path/dir/a/b/c/file3.txt",
				"/path/dir/a/c/file4.txt",
			})

			treeDBPath := createTree(t, treeDB)
			So(root.AddTree(treeDBPath), ShouldBeNil)

			r1 := createRule(t, tdb, root, "/path/dir/", "b/c/*", true)
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{0: 3, r1: 1})
			So(ruleIDCount(t, root, "/path/dir/a/"), ShouldResemble, map[uint64]uint64{0: 3, r1: 1})
			So(ruleIDCount(t, root, "/path/dir/a/b/"), ShouldResemble, map[uint64]uint64{0: 1, r1: 1})
			So(ruleIDCount(t, root, "/path/dir/a/b/c/"), ShouldResemble, map[uint64]uint64{r1: 1})
			So(ruleIDCount(t, root, "/path/dir/a/c/"), ShouldResemble, map[uint64]uint64{0: 1})

			r2 := createRule(t, tdb, root, "/path/dir/a/", "*")
			So(ruleIDCount(t, root, "/path/dir/"), ShouldResemble, map[uint64]uint64{r1: 1, r2: 3})
			So(ruleIDCount(t, root, "/path/dir/a/"), ShouldResemble, map[uint64]uint64{r1: 1, r2: 3})
			So(ruleIDCount(t, root, "/path/dir/a/b/"), ShouldResemble, map[uint64]uint64{r1: 1, r2: 1})
			So(ruleIDCount(t, root, "/path/dir/a/b/c/"), ShouldResemble, map[uint64]uint64{r1: 1})
			So(ruleIDCount(t, root, "/path/dir/a/c/"), ShouldResemble, map[uint64]uint64{r2: 1})

			r3 := createRule(t, tdb, root, "/path/dir/a/b/c/", "*.txt")
			So(ruleIDCount(t, root, "/path/dir/a/"), ShouldResemble, map[uint64]uint64{r2: 3, r3: 1})
			So(ruleIDCount(t, root, "/path/dir/a/b/"), ShouldResemble, map[uint64]uint64{r2: 1, r3: 1})
			So(ruleIDCount(t, root, "/path/dir/a/b/c/"), ShouldResemble, map[uint64]uint64{r3: 1})
		})

		Convey("When adding a rule in a directory, all non-affected directory summaries are correctly copied", func() {
			_, dbPath := plandb.PopulateBigExamplePlanDB(t)
			treeDB := plandb.ExampleTreeBig()

			rulesDB, err := db.Init(dbPath)
			So(err, ShouldBeNil)

			dirs := make(map[int64]*db.Directory)
			dirRules := make([]DirRule, 0)

			So(rulesDB.ReadDirectories().ForEach(func(dir *db.Directory) error {
				dirs[dir.ID()] = dir

				return nil
			}), ShouldBeNil)

			So(rulesDB.ReadRules().ForEach(func(r *db.Rule) error {
				dirRules = append(dirRules, DirRule{
					Directory: dirs[r.DirID()],
					Rule:      r,
				})

				return nil
			}), ShouldBeNil)

			root, err := NewRoot(dirRules)
			So(err, ShouldBeNil)

			treeDBPath := createTree(t, treeDB)
			So(root.AddTree(treeDBPath), ShouldBeNil)

			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/"), ShouldResemble, map[uint64]uint64{0: 4, 1: 2, 2: 1, 3: 3, 4: 1, 5: 2, 6: 2}) //nolint:lll
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/b/"), ShouldResemble, map[uint64]uint64{0: 3, 1: 2, 2: 1, 4: 1, 5: 2})           //nolint:lll
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/b/newdir/"), ShouldResemble, map[uint64]uint64{4: 1, 5: 2})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/b/testdir/"), ShouldResemble, map[uint64]uint64{0: 1})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/c/"), ShouldResemble, map[uint64]uint64{3: 3, 6: 2})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/c/newdir/"), ShouldResemble, map[uint64]uint64{3: 2, 6: 1})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/d/"), ShouldResemble, map[uint64]uint64{0: 1})

			r1 := createRule(t, rulesDB, root, "/lustre/scratch123/humgen/a/b/testdir/", "*")
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/"), ShouldResemble, map[uint64]uint64{0: 3, 1: 2, 2: 1, 3: 3, 4: 1, 5: 2, 6: 2, r1: 1}) //nolint:lll
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/b/"), ShouldResemble, map[uint64]uint64{0: 2, 1: 2, 2: 1, 4: 1, 5: 2, r1: 1})           //nolint:lll
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/b/newdir/"), ShouldResemble, map[uint64]uint64{4: 1, 5: 2})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/b/testdir/"), ShouldResemble, map[uint64]uint64{r1: 1})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/c/"), ShouldResemble, map[uint64]uint64{3: 3, 6: 2})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/c/newdir/"), ShouldResemble, map[uint64]uint64{3: 2, 6: 1})
			So(ruleIDCount(t, root, "/lustre/scratch123/humgen/a/d/"), ShouldResemble, map[uint64]uint64{0: 1})
		})
	})
}

func buildTreeDB(t *testing.T, files []string) *directories.Root {
	t.Helper()

	const rootPath = "/path/"

	treeDB := directories.NewRoot(rootPath, time.Now().Unix())

	for _, file := range files {
		directories.AddFile(&treeDB.Directory, strings.TrimPrefix(file, rootPath), 1, 2, 3, 4)
	}

	return treeDB
}

func createTree(t *testing.T, node tree.Node) string {
	t.Helper()

	treeDBPath := filepath.Join(t.TempDir(), "tree.db")

	f, err := os.Create(treeDBPath)
	So(err, ShouldBeNil)
	So(tree.Serialise(f, node), ShouldBeNil)
	So(f.Close(), ShouldBeNil)

	return treeDBPath
}

func RemoveRule(t *testing.T, tdb *db.DB, root *RootDir, dirPath, match string) {
	t.Helper()

	directory := getDir(t, tdb, dirPath)
	So(directory, ShouldNotBeNil)

	rule := getRule(t, tdb, directory, match)

	So(root.RemoveRule(directory, rule), ShouldBeNil)
}

func getRule(t *testing.T, tdb *db.DB, directory *db.Directory, match string) *db.Rule {
	t.Helper()

	var foundRule *db.Rule

	So(tdb.ReadRules().ForEach(func(rule *db.Rule) error {
		if rule.Match == match && rule.DirID() == directory.ID() {
			foundRule = rule
		}

		return nil
	}), ShouldBeNil)

	So(foundRule, ShouldNotBeNil)

	return foundRule
}

func getDir(t *testing.T, tdb *db.DB, dirPath string) *db.Directory {
	t.Helper()

	var directory *db.Directory

	So(tdb.ReadDirectories().ForEach(func(dir *db.Directory) error {
		if dir.Path == dirPath {
			directory = dir
		}

		return nil
	}), ShouldBeNil)

	return directory
}

func createRule(t *testing.T, tdb *db.DB, root *RootDir, dirPath, match string, override ...bool) uint64 {
	t.Helper()

	directory := getDir(t, tdb, dirPath)

	if directory == nil {
		directory = &db.Directory{Path: dirPath}

		So(tdb.CreateDirectory(directory), ShouldBeNil)
	}

	r := &db.Rule{Match: match, Override: len(override) > 0 && override[0]}

	So(tdb.CreateDirectoryRule(directory, r), ShouldBeNil)
	So(root.AddRule(directory, r), ShouldBeNil)

	return uint64(r.ID()) //nolint:gosec
}

func genRules(t *testing.T, tdb *db.DB, rs map[string][]string) []DirRule {
	t.Helper()

	var rules []DirRule

	paths := slices.Collect(maps.Keys(rs))

	slices.Sort(paths)

	for _, path := range paths {
		matches := rs[path]

		dir := &db.Directory{Path: path}

		So(tdb.CreateDirectory(dir), ShouldBeNil)

		for _, match := range matches {
			rule := &db.Rule{Match: match}

			So(tdb.CreateDirectoryRule(dir, rule), ShouldBeNil)

			rules = append(rules, DirRule{Directory: dir, Rule: rule})
		}
	}

	slices.SortFunc(rules, func(a, b DirRule) int {
		cmp := strings.Compare(a.Path, b.Path)
		if cmp == 0 {
			cmp = strings.Compare(a.Match, b.Match)
		}

		return cmp
	})

	return rules
}

func ruleIDCount(t *testing.T, root *RootDir, path string) map[uint64]uint64 {
	t.Helper()

	s, err := root.Summary(path)
	So(err, ShouldBeNil)

	rules := make(map[uint64]uint64)

	for _, rs := range s.RuleSummaries {
		for _, r := range rs.Users {
			rules[rs.ID] += r.Files
		}
	}

	return rules
}
