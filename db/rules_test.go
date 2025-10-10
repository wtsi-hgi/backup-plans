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

package db

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRules(t *testing.T) {
	Convey("With a test database", t, func() {
		db := createTestDatabase(t)

		dirA := &Directory{
			Path:      "/some/path/",
			ClaimedBy: "me",
		}
		dirB := &Directory{
			Path:      "/some/other/path/",
			ClaimedBy: "someone",
		}

		So(db.CreateDirectory(dirA), ShouldBeNil)
		So(db.CreateDirectory(dirB), ShouldBeNil)

		Convey("You can add rules to a directory", func() {
			ruleA := &Rule{
				BackupType: BackupIBackup,
				Match:      "*.jpg",
				Frequency:  7,
			}
			ruleB := &Rule{
				BackupType: BackupTemp,
				Match:      "*.log",
				Frequency:  1,
			}
			ruleC := &Rule{
				BackupType: BackupNone,
				Match:      "*.jpg",
				Frequency:  7,
			}

			So(db.CreateDirectoryRule(dirA, ruleA), ShouldBeNil)
			So(db.CreateDirectoryRule(dirA, ruleB), ShouldBeNil)
			So(db.CreateDirectoryRule(dirA, ruleC), ShouldNotBeNil)
			So(db.CreateDirectoryRule(dirB, ruleC), ShouldBeNil)

			Convey("…and retrieve them from the DB", func() {
				So(collectIter(t, db.ReadRules()), ShouldResemble, []*Rule{ruleA, ruleB, ruleC})
			})

			Convey("…and update them", func() {
				ruleA.Frequency = 14

				So(db.UpdateRule(ruleA), ShouldBeNil)
				So(collectIter(t, db.ReadRules()), ShouldResemble, []*Rule{ruleA, ruleB, ruleC})
			})

			Convey("…and remove them", func() {
				So(db.RemoveRule(ruleA), ShouldBeNil)
				So(collectIter(t, db.ReadRules()), ShouldResemble, []*Rule{ruleB, ruleC})
			})

			Convey("…and removing a directory removes all of its rules", func() {
				So(db.RemoveDirectory(dirA), ShouldBeNil)
				So(collectIter(t, db.ReadRules()), ShouldResemble, []*Rule{ruleC})
			})
		})
	})
}
