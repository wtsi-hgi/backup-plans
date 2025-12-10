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
		}

		dirRules := []DirRule{
			{Directory: &dirs[0], Rule: &db.Rule{Match: "*"}},
			{Directory: &dirs[0], Rule: &db.Rule{Match: "*.txt"}},
			{Directory: &dirs[1], Rule: &db.Rule{Match: "*"}},
			{Directory: &dirs[2], Rule: &db.Rule{Match: "a*"}},
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
		})
	})
}
