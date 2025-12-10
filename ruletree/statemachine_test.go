package ruletree

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
)

func TestStateMachine(t *testing.T) {
	Convey("With a RootDir containing rules", t, func() {
		tdb := testdb.CreateTestDatabase(t)

		dirs := []db.Directory{
			{Path: "/some/path/MyDir/"},
			{Path: "/some/path/YourDir/"},
			{Path: "/some/path/AnotherDir/"},
			{Path: "/slash/check/"},
			{Path: "/slash/check/override/"},
			{Path: "/slash/check/override/child/"},
			{Path: "/parent/override/"},
			{Path: "/parent/override/child/"},
			{Path: "/parent/override/deep/child/"},
			{Path: "/parent/override/some/child/"},
		}

		dirRules := []DirRule{
			{Directory: &dirs[0], Rule: &db.Rule{Match: "*"}},
			{Directory: &dirs[0], Rule: &db.Rule{Match: "*.txt"}},
			{Directory: &dirs[1], Rule: &db.Rule{Match: "*"}},
			{Directory: &dirs[2], Rule: &db.Rule{Match: "a*"}},
			{Directory: &dirs[2], Rule: &db.Rule{Match: "c*.txt"}},
			{Directory: &dirs[3], Rule: &db.Rule{Match: "with/slash"}},
			{Directory: &dirs[4], Rule: &db.Rule{Match: "child/slash"}},
			{Directory: &dirs[5], Rule: &db.Rule{Match: "slash"}},
			{Directory: &dirs[6], Rule: &db.Rule{Match: "bad/*", Override: true}},
			{Directory: &dirs[7], Rule: &db.Rule{Match: "*"}},
			{Directory: &dirs[8], Rule: &db.Rule{Match: "*"}},
			{Directory: &dirs[9], Rule: &db.Rule{Match: "bad/*"}},
		}

		for _, dr := range dirRules {
			if dr.Directory.ID() == 0 {
				So(tdb.CreateDirectory(dr.Directory), ShouldBeNil)
			}

			So(tdb.CreateDirectoryRule(dr.Directory, dr.Rule), ShouldBeNil)
		}

		root, err := NewRoot(dirRules)
		So(err, ShouldBeNil)

		Convey("You can build and test a statemachine", func() {
			sm, err := root.generateStatemachineFor("/", nil)
			So(err, ShouldBeNil)

			So(*sm.GetStateString("/").GetGroup(), ShouldEqual, processRules)
			So(*sm.GetStateString("/some/non/path/").GetGroup(), ShouldEqual, 0)
			So(*sm.GetStateString("/some/path/").GetGroup(), ShouldEqual, processRules)
			So(*sm.GetStateString("/some/path/MyDir/").GetGroup(), ShouldEqual, processRules)
			So(*sm.GetStateString("/some/path/YourDir/").GetGroup(), ShouldEqual, dirRules[2].Rule.ID())
			So(*sm.GetStateString("/some/path/AnotherDir/").GetGroup(), ShouldEqual, processRules)
			So(*sm.GetStateString("/some/path/AnotherDir/a.txt").GetGroup(), ShouldEqual, dirRules[3].Rule.ID())
			So(*sm.GetStateString("/some/path/AnotherDir/a/").GetGroup(), ShouldEqual, dirRules[3].Rule.ID())
			So(*sm.GetStateString("/some/path/AnotherDir/a/b.txt").GetGroup(), ShouldEqual, dirRules[3].Rule.ID())
			So(*sm.GetStateString("/some/path/AnotherDir/b/").GetGroup(), ShouldEqual, 0)
			So(*sm.GetStateString("/some/path/AnotherDir/b/a.txt").GetGroup(), ShouldEqual, 0)
			So(*sm.GetStateString("/some/path/AnotherDir/c/a.txt").GetGroup(), ShouldEqual, dirRules[4].Rule.ID())
			So(*sm.GetStateString("/some/path/AnotherDir/c/").GetGroup(), ShouldEqual, processRules)
			So(*sm.GetStateString("/some/path/AnotherDir/c/with/").GetGroup(), ShouldEqual, processRules)

			So(*sm.GetStateString("/slash/check/with/slash").GetGroup(), ShouldEqual, dirRules[5].Rule.ID())
			So(*sm.GetStateString("/slash/check/override/child/slash").GetGroup(), ShouldEqual, dirRules[7].Rule.ID())

			So(*sm.GetStateString("/parent/override/bad/1").GetGroup(), ShouldEqual, dirRules[8].Rule.ID())
			So(*sm.GetStateString("/parent/override/other/bad").GetGroup(), ShouldEqual, 0)
			So(*sm.GetStateString("/parent/override/other/bad/1").GetGroup(), ShouldEqual, dirRules[8].Rule.ID())
			So(*sm.GetStateString("/parent/override/child/bad").GetGroup(), ShouldEqual, dirRules[9].Rule.ID())
			So(*sm.GetStateString("/parent/override/child/bad/1").GetGroup(), ShouldEqual, dirRules[8].Rule.ID())
			So(*sm.GetStateString("/parent/override/deep/child/bad").GetGroup(), ShouldEqual, dirRules[10].Rule.ID())
			So(*sm.GetStateString("/parent/override/deep/child/bad/1").GetGroup(), ShouldEqual, dirRules[8].Rule.ID())
			So(*sm.GetStateString("/parent/override/deep/child/deeper/bad/1").GetGroup(), ShouldEqual, dirRules[8].Rule.ID())
			So(*sm.GetStateString("/parent/override/some/child/bad/1").GetGroup(), ShouldEqual, dirRules[11].Rule.ID())
			So(*sm.GetStateString("/parent/override/some/child/bad/bad/1").GetGroup(), ShouldEqual, dirRules[8].Rule.ID())
		})
	})
}
