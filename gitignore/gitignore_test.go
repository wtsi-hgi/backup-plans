package gitignore

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNew(t *testing.T) {
	Convey("Given a gitIgnore filepath, a gitignore object with a valid matcher can be retrieved", t, func() {
		gi, err := New("gitignoreExample.txt")
		So(gi, ShouldNotBeNil)
		So(err, ShouldBeNil)

		Convey("Files can be split into ignore/keep", func() {
			paths := []string{"testfile.txt", "testfile2.log"}
			ignore, keep := gi.Match(paths)
			So(ignore, ShouldEqual, []string{"testfile2.log"})
			So(keep, ShouldEqual, []string{"testfile.txt"}) //TODO: check with CASM what behaviour they have in mind for files not matched by any rule
		})

		Convey("Rules can be retrieved", func() {
			rules, err := gi.GetRules()
			So(err, ShouldBeNil)
			So(rules, ShouldEqual, []string{"*.log", "/build/", "/!important.log"})
		})

		Convey("Rules can be added", func() {
			err = gi.AddRules([]string{"*.txt", "/test/"})
			So(err, ShouldBeNil)
			newrules, err := gi.GetRules()
			So(err, ShouldBeNil)
			So(newrules, ShouldEqual, []string{"*.log", "/build/", "/!important.log", "*.txt", "/test/"})
		})

		Convey("Rules can be removed", func() {
			err = gi.RemoveRules([]string{"*.log"})
			So(err, ShouldBeNil)

			newrules, err := gi.GetRules()
			So(err, ShouldBeNil)
			So(newrules, ShouldEqual, []string{"/build/", "/!important.log"})
		})
	})
}
