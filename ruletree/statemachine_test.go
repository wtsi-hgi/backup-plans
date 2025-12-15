package ruletree

import (
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
)

func TestStateMachine(t *testing.T) {
	Convey("With a RootDir containing rules", t, func() {
		tdb := testdb.CreateTestDatabase(t)

		root, err := NewRoot(createDirRules(t, tdb, map[string][]*db.Rule{
			"/some/path/MyDir/": {
				{Match: "*"},
				{Match: "*.txt"},
			},
			"/some/path/YourDir/": {
				{Match: "*"},
			},
			"/some/path/AnotherDir/": {
				{Match: "a*"},
				{Match: "c*.txt"},
			},
			"/slash/check/": {
				{Match: "with/slash"},
			},
			"/slash/check/override/": {
				{Match: "child/slash"},
			},
			"/slash/check/override/child/": {
				{Match: "slash"},
			},
			"/parent/override/": {
				{Match: "bad/*", Override: true},
			},
			"/parent/override/child/": {
				{Match: "*"},
			},
			"/parent/override/deep/child/": {
				{Match: "*"},
			},
			"/parent/override/some/child/": {
				{Match: "bad/*"},
			},
		}))
		So(err, ShouldBeNil)

		Convey("You can build and test a statemachine", func() {
			sm, err := root.generateStatemachineFor("/", nil)
			So(err, ShouldBeNil)

			for n, test := range [...]struct {
				Path, Dir, Match       string
				Process, NoRules, Copy bool
			}{
				{Path: "/", Process: true},
				{Path: "/some/non/path/", NoRules: true},
				{Path: "/some/path/", Process: true},
				{Path: "/some/path/MyDir/", Process: true},
				{Path: "/some/path/YourDir/", Dir: "/some/path/YourDir/", Match: "*"},
				{Path: "/some/path/AnotherDir/", Process: true},
				{Path: "/some/path/AnotherDir/a.txt", Dir: "/some/path/AnotherDir/", Match: "a*"},
				{Path: "/some/path/AnotherDir/a/", Dir: "/some/path/AnotherDir/", Match: "a*"},
				{Path: "/some/path/AnotherDir/a/b.txt", Dir: "/some/path/AnotherDir/", Match: "a*"},
				{Path: "/some/path/AnotherDir/b/", NoRules: true},
				{Path: "/some/path/AnotherDir/b/a.txt", NoRules: true},
				{Path: "/some/path/AnotherDir/c/a.txt", Dir: "/some/path/AnotherDir/", Match: "c*.txt"},
				{Path: "/some/path/AnotherDir/c/", Process: true},
				{Path: "/some/path/AnotherDir/c/with/", Process: true},

				{Path: "/slash/check/with/slash", Dir: "/slash/check/", Match: "with/slash"},
				{Path: "/slash/check/override/child/slash", Dir: "/slash/check/override/child/", Match: "slash"},

				{Path: "/parent/override/bad/1", Dir: "/parent/override/", Match: "bad/*"},
				{Path: "/parent/override/other/bad", NoRules: true},
				{Path: "/parent/override/other/bad/1", Dir: "/parent/override/", Match: "bad/*"},
				{Path: "/parent/override/child/bad", Dir: "/parent/override/child/", Match: "*"},
				{Path: "/parent/override/child/bad/1", Dir: "/parent/override/", Match: "bad/*"},
				{Path: "/parent/override/deep/child/bad", Dir: "/parent/override/deep/child/", Match: "*"},
				{Path: "/parent/override/deep/child/bad/1", Dir: "/parent/override/", Match: "bad/*"},
				{Path: "/parent/override/deep/child/deeper/bad/1", Dir: "/parent/override/", Match: "bad/*"},
				{Path: "/parent/override/some/child/bad/1", Dir: "/parent/override/some/child/", Match: "bad/*"},
				{Path: "/parent/override/some/child/bad/bad/1", Dir: "/parent/override/", Match: "bad/*"},
			} {
				var id int64

				if test.Process {
					id = processRules
				} else if !test.NoRules {
					id = getRule(t, tdb, getDir(t, tdb, test.Dir), test.Match).ID()
				}

				if test.Copy {
					id = -id
				}

				SoMsg("Test: "+strconv.Itoa(n+1), *sm.GetStateString(test.Path).GetGroup(), ShouldEqual, id)
			}
		})
	})
}

func createDirRules(t *testing.T, tdb *db.DB, rules map[string][]*db.Rule) []DirRule {
	var dirRules []DirRule

	for dir, rs := range rules {
		directory := getDir(t, tdb, dir)

		if directory == nil {
			directory = &db.Directory{Path: dir}

			So(tdb.CreateDirectory(directory), ShouldBeNil)
		}

		for _, r := range rs {
			So(tdb.CreateDirectoryRule(directory, r), ShouldBeNil)

			dirRules = append(dirRules, DirRule{Directory: directory, Rule: r})
		}
	}

	return dirRules
}
