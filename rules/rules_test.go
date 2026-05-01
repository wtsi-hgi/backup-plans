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
	"time"

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
			So(rules.dirs[1], ShouldEqual, rules.directoryRules["/path/to/myDir/"])

			So(collectDirs(rules), ShouldResemble, []Directory{
				{Path: "/path/to/myDir/", ClaimedBy: "me", Frequency: 7},
			})

			So(rules.ClaimDirectory("/path/to/myOtherDir/", "me"), ShouldBeNil)

			So(collectDirs(rules), ShouldResemble, []Directory{
				{Path: "/path/to/myDir/", ClaimedBy: "me", Frequency: 7},
				{Path: "/path/to/myOtherDir/", ClaimedBy: "me", Frequency: 7},
			})

			So(rules.PassDirectory("/path/to/otherDir/", "you"), ShouldEqual, ErrDirectoryNotClaimed)
			So(rules.PassDirectory("/path/to/myDir/", "you"), ShouldBeNil)

			So(collectDirs(rules), ShouldResemble, []Directory{
				{Path: "/path/to/myDir/", ClaimedBy: "you", Frequency: 7},
				{Path: "/path/to/myOtherDir/", ClaimedBy: "me", Frequency: 7},
			})

			So(rules.ForfeitDirectory("/path/to/otherDir/"), ShouldEqual, ErrDirectoryNotClaimed)

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

		Convey("You can set and retrive the details for a claimed directory", func() {
			now := time.Now()

			So(rules.DirDetails("/path/to/myDir/"), ShouldBeNil)
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldBeNil)

			dir := rules.DirDetails("/path/to/myDir/")
			So(dir.ReviewDate, ShouldBeBetweenOrEqual, now.Add(twoyears).Unix(), time.Now().Add(twoyears).Unix()+1)
			So(dir.RemoveDate, ShouldBeBetweenOrEqual, now.Add(twoyears).Add(month).Unix(), time.Now().Add(twoyears).Add(month).Unix()+1)

			So(dir, ShouldResemble, &Directory{
				Path:       "/path/to/myDir/",
				ClaimedBy:  "me",
				Frequency:  defaultFrequency,
				ReviewDate: dir.ReviewDate,
				RemoveDate: dir.RemoveDate,
			})

			So(rules.SetDirDetails(Directory{
				Path:       "/path/to/myDir/",
				Frequency:  100,
				ReviewDate: 1000,
				RemoveDate: 2000,
			}), ShouldBeNil)
			So(rules.DirDetails("/path/to/myDir/"), ShouldResemble, &Directory{
				Path:       "/path/to/myDir/",
				ClaimedBy:  "me",
				Frequency:  100,
				ReviewDate: 1000,
				RemoveDate: 2000,
			})
		})

		Convey("You can add, edit, and remove rules to claimed directories", func() {
			So(rules.HasRules("/path/to/myDir/"), ShouldBeFalse)
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldBeNil)
			So(len(rules.dirs[1].Rules), ShouldBeZeroValue)
			So(rules.rules[1], ShouldBeNil)
			So(rules.Rule(1), ShouldBeNil)
			So(rules.HasRules("/path/to/myDir/"), ShouldBeFalse)

			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupIBackup,
				Metadata:   "meta",
				Override:   true,
			}), ShouldBeNil)
			So(len(rules.dirs[1].Rules), ShouldEqual, 1)
			So(rules.dirs[1].Rules["*.txt"], ShouldNotBeNil)
			So(rules.rules[1], ShouldNotBeNil)
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
			So(rules.Rule(1), ShouldResemble, &Rule{
				ID:          1,
				DirectoryID: 1,
				Match:       "*.txt",
				BackupType:  db.BackupIBackup,
				Metadata:    "meta",
				Override:    true,
			})
			So(rules.HasRules("/path/to/myDir/"), ShouldBeTrue)

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

			So(rules.UpdateRule("/path/to/myDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupManualIBackup,
				Metadata:   "aaa",
			}), ShouldBeNil)
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

			So(rules.UpdateRule("/path/to/myDir/", Rule{
				Match:      "*.none",
				BackupType: db.BackupManualIBackup,
				Metadata:   "aaa",
			}), ShouldEqual, ErrNoRule)
			So(rules.UpdateRule("/path/to/otherDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupManualIBackup,
				Metadata:   "aaa",
			}), ShouldEqual, ErrDirectoryNotClaimed)

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

		Convey("Forfeiting a directory that has rules on it also removes the rules", func() {
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldBeNil)
			So(rules.rules[1], ShouldBeNil)
			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupIBackup,
				Metadata:   "meta",
				Override:   true,
			}), ShouldBeNil)
			So(rules.rules[1], ShouldNotBeNil)
			So(rules.ForfeitDirectory("/path/to/myDir/"), ShouldBeNil)
			So(rules.rules[1], ShouldBeNil)
		})

		Convey("Transactions allow you to add or remove rules without affecting readers until Commit", func() {
			So(rules.ClaimDirectory("/path/to/myDir/", "me"), ShouldBeNil)

			So(rules.AddRules("/path/to/myDir/", Rule{
				Match:      "*.txt",
				BackupType: db.BackupIBackup,
				Metadata:   "meta",
				Override:   true,
			}), ShouldBeNil)
			So(len(rules.rules), ShouldEqual, 1)

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
			So(len(rules.rules), ShouldEqual, 1)

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
			So(len(rules.rules), ShouldEqual, 2)

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
			So(len(rules.rules), ShouldEqual, 2)

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
			So(len(rules.rules), ShouldEqual, 1)

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
