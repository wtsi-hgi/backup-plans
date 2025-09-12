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
