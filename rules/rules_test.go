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

package rules

import (
	"slices"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
)

func TestRules(t *testing.T) {
	Convey("With a test DB", t, func() {
		rdb := testdb.CreateTestDatabase(t)

		rules, err := New(rdb)
		So(err, ShouldBeNil)

		Convey("You can claim, pass, and forfeit directories", func() {
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldBeNil)
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldEqual, ErrDirectoryClaimed)
			So(rules.ClaimDirectory("/path/to/myDir/", "you"), ShouldEqual, ErrDirectoryClaimed)

			So(collectDirs(rules), ShouldResemble, []Directory{
				{Path: "/path/to/myDir/", ClaimedBy: "me", Frequency: 7},
			})

			So(rules.ClaimDirectory("/path/to/myOtherDir/", "me"), ShouldBeNil)

			So(collectDirs(rules), ShouldResemble, []Directory{
				{Path: "/path/to/myDir/", ClaimedBy: "me", Frequency: 7},
				{Path: "/path/to/myOtherDir/", ClaimedBy: "me", Frequency: 7},
			})

			So(rules.PassDirectory("/path/to/myDir/", "you"), ShouldBeNil)

			So(collectDirs(rules), ShouldResemble, []Directory{
				{Path: "/path/to/myDir/", ClaimedBy: "you", Frequency: 7},
				{Path: "/path/to/myOtherDir/", ClaimedBy: "me", Frequency: 7},
			})

			So(rules.ForfeitDirectory("/path/to/myDir/"), ShouldBeNil)
			So(collectDirs(rules), ShouldResemble, []Directory{
				{Path: "/path/to/myOtherDir/", ClaimedBy: "me", Frequency: 7},
			})

			Convey("A new Database using the same underlying DB should have the same directory claimed", func() {
				newRules, err := New(rdb)
				So(err, ShouldBeNil)

				So(collectDirs(newRules), ShouldResemble, []Directory{
					{Path: "/path/to/myOtherDir/", ClaimedBy: "me", Frequency: 7},
				})
			})
		})

		Convey("You can add, edit, and remove rules to claimed directories", func() {
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldBeNil)

			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupIBackup,
				Metadata:   "meta",
				Override:   true,
			}), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupIBackup,
						Metadata:    "meta",
						Override:    true,
					},
				},
			})

			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupManualUnchecked,
				Metadata:   "meta",
				Override:   true,
			}), ShouldEqual, ErrRuleExists)
			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.zip",
				BackupType: db.BackupManualUnchecked,
			}, Rule{
				Match:      "*.zip",
				BackupType: db.BackupManualUnchecked,
			}), ShouldEqual, ErrDuplicateRule)
			So(rules.AddRules("/path/to/otherDir/", Rule{
				Match:      "*.zip",
				BackupType: db.BackupManualUnchecked,
			}), ShouldEqual, ErrDirectoryNotClaimed)
			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.zip",
				BackupType: db.BackupManualUnchecked,
			}), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupIBackup,
						Metadata:    "meta",
						Override:    true,
					},
					{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.zip",
						BackupType:  db.BackupManualUnchecked,
					},
				},
			})

			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.gz",
				BackupType: db.BackupManualUnchecked,
			}, Rule{
				Match:      "temp.*",
				BackupType: db.BackupManualUnchecked,
			}), ShouldBeNil)

			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupIBackup,
						Metadata:    "meta",
						Override:    true,
					},
					{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.zip",
						BackupType:  db.BackupManualUnchecked,
					},
					{
						ID:          3,
						DirectoryID: 1,
						Match:       "*.gz",
						BackupType:  db.BackupManualUnchecked,
					},
					{
						ID:          4,
						DirectoryID: 1,
						Match:       "temp.*",
						BackupType:  db.BackupManualUnchecked,
					},
				},
			})

			So(rules.UpdateRule("/path/to/myDir/", Rule{Match: "*.txt", BackupType: db.BackupManualIBackup, Metadata: "aaa"}), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupManualIBackup,
						Metadata:    "aaa",
					},
					{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.zip",
						BackupType:  db.BackupManualUnchecked,
					},
					{
						ID:          3,
						DirectoryID: 1,
						Match:       "*.gz",
						BackupType:  db.BackupManualUnchecked,
					},
					{
						ID:          4,
						DirectoryID: 1,
						Match:       "temp.*",
						BackupType:  db.BackupManualUnchecked,
					},
				},
			})

			So(rules.UpdateRule("/path/to/myDir/", Rule{Match: "*.none", BackupType: db.BackupManualIBackup, Metadata: "aaa"}), ShouldEqual, ErrNoRule)
			So(rules.UpdateRule("/path/to/otherDir/", Rule{Match: "*.txt", BackupType: db.BackupManualIBackup, Metadata: "aaa"}), ShouldEqual, ErrDirectoryNotClaimed)

			So(rules.RemoveRules("/path/to/myDir/", "*.txt"), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.zip",
						BackupType:  db.BackupManualUnchecked,
					},
					{
						ID:          3,
						DirectoryID: 1,
						Match:       "*.gz",
						BackupType:  db.BackupManualUnchecked,
					},
					{
						ID:          4,
						DirectoryID: 1,
						Match:       "temp.*",
						BackupType:  db.BackupManualUnchecked,
					},
				},
			})

			So(rules.RemoveRules("/path/to/myDir/", "*.txt"), ShouldEqual, ErrNoRule)
			So(rules.RemoveRules("/path/to/myDir/", "*.zip", "*.other"), ShouldEqual, ErrNoRule)
			So(rules.RemoveRules("/path/to/otherDir/", "*.zip", "temp.*"), ShouldEqual, ErrDirectoryNotClaimed)
			So(rules.RemoveRules("/path/to/myDir/", "*.zip", "temp.*"), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          3,
						DirectoryID: 1,
						Match:       "*.gz",
						BackupType:  db.BackupManualUnchecked,
					},
				},
			})

			Convey("A new Database using the same underlying DB should have the same rules", func() {
				newRules, err := New(rdb)
				So(err, ShouldBeNil)

				So(collectRules(newRules), ShouldResemble, map[string][]Rule{
					"/path/to/myDir/": {
						{
							ID:          3,
							DirectoryID: 1,
							Match:       "*.gz",
							BackupType:  db.BackupManualUnchecked,
						},
					},
				})
			})
		})

		Convey("Transactions allow you to add or remove rules without affecting readers until Commit", func() {
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldBeNil)

			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupIBackup,
				Metadata:   "meta",
				Override:   true,
			}), ShouldBeNil)

			tx := rules.RuleTransaction()

			So(tx.AddRules("/path/to/myDir/", Rule{
				Match:      "*.csv",
				BackupType: db.BackupIBackup,
			}), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupIBackup,
						Metadata:    "meta",
						Override:    true,
					},
				},
			})
			So(collectRules(tx), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupIBackup,
						Metadata:    "meta",
						Override:    true,
					},
					Rule{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.csv",
						BackupType:  db.BackupIBackup,
					},
				},
			})

			So(tx.Commit(), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupIBackup,
						Metadata:    "meta",
						Override:    true,
					},
					Rule{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.csv",
						BackupType:  db.BackupIBackup,
					},
				},
			})

			tx = rules.RuleTransaction()
			So(tx.RemoveRules("/path/to/myDir/", "*.txt"), ShouldBeNil)
			So(collectRules(tx), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					Rule{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.csv",
						BackupType:  db.BackupIBackup,
					},
				},
			})
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					{
						ID:          1,
						DirectoryID: 1,
						Match:       "*.txt",
						BackupType:  db.BackupIBackup,
						Metadata:    "meta",
						Override:    true,
					},
					Rule{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.csv",
						BackupType:  db.BackupIBackup,
					},
				},
			})

			So(tx.Commit(), ShouldBeNil)
			So(collectRules(rules), ShouldResemble, map[string][]Rule{
				"/path/to/myDir/": {
					Rule{
						ID:          2,
						DirectoryID: 1,
						Match:       "*.csv",
						BackupType:  db.BackupIBackup,
					},
				},
			})

			Convey("Rolling back a transaction keeps the database at its previous state", func() {
				tx := rules.RuleTransaction()

				So(tx.AddRules("/path/to/myDir/", Rule{
					Match:      "*.txt",
					BackupType: db.BackupIBackup,
				}), ShouldBeNil)
				So(tx.Rollback(), ShouldBeNil)
				So(collectRules(rules), ShouldResemble, map[string][]Rule{
					"/path/to/myDir/": {
						Rule{
							ID:          2,
							DirectoryID: 1,
							Match:       "*.csv",
							BackupType:  db.BackupIBackup,
						},
					},
				})

				tx = rules.RuleTransaction()

				So(tx.RemoveRules("/path/to/myDir/", "*.csv"), ShouldBeNil)
				So(tx.Rollback(), ShouldBeNil)
				So(collectRules(rules), ShouldResemble, map[string][]Rule{
					"/path/to/myDir/": {
						Rule{
							ID:          2,
							DirectoryID: 1,
							Match:       "*.csv",
							BackupType:  db.BackupIBackup,
						},
					},
				})
			})
		})
	})
}

func collectDirs(rules *Database) []Directory {
	dirs := slices.Collect(rules.Dirs())

	slices.SortFunc(dirs, func(a, b Directory) int { return strings.Compare(a.Path, b.Path) })

	for n := range dirs {
		dirs[n].ReviewDate = 0
		dirs[n].RemoveDate = 0
	}

	return dirs
}

func collectRules(rules *Database) map[string][]Rule {
	rs := make(map[string][]Rule)

	for dir := range rules.Dirs() {
		ruleList := slices.Collect(rules.DirRules(dir.Path))

		slices.SortFunc(ruleList, func(a, b Rule) int { return int(a.ID) - int(b.ID) })

		rs[dir.Path] = ruleList
	}

	return rs
}
