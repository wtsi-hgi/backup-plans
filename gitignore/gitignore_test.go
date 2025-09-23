package gitignore

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNew(t *testing.T) {
	Convey("Given a gitIgnore filepath, a gitignore object can be retrieved", t, func() {
		gi, err := New("gitignoreExample.txt")
		So(gi, ShouldNotBeNil)
		So(err, ShouldBeNil)

		Convey("Files can be split into ignore/keep", func() {
			paths := []string{"testfile.txt", "testfile2.log"}
			ignore, keep := gi.Match(paths)
			So(ignore, ShouldResemble, []string{"testfile2.log"})
			So(keep, ShouldResemble, []string{"testfile.txt"})
		})

		Convey("Rules can be retrieved", func() {
			rules, err := gi.GetRules()
			So(err, ShouldBeNil)
			So(rules, ShouldEqual, []string{"*.log", "/build/", "/!important.log"})
		})

		Convey("Rules can be added", func() {
			newrules, err := gi.AddRules([]string{"*.txt", "/test/"})
			So(err, ShouldBeNil)
			So(newrules, ShouldEqual, []string{"*.log", "/build/", "/!important.log", "*.txt", "/test/"})

			Convey("With the insertion indices specified", func() {
				newrules, err = gi.AddRulesAt([]string{"*.png", "/test2/"}, []int{1, 3})
				So(err, ShouldBeNil)
				So(newrules, ShouldEqual, []string{"*.log", "*.png", "/build/", "/test2/", "/!important.log", "*.txt", "/test/"})
				newrules, err = gi.AddRulesAt([]string{"/ignoreme/"}, []int{0})
				So(err, ShouldBeNil)
				So(newrules, ShouldEqual, []string{"/ignoreme/", "*.log", "*.png", "/build/", "/test2/", "/!important.log", "*.txt", "/test/"})
				newrules, err = gi.AddRulesAt([]string{"/ignoremepls/"}, []int{8})
				So(err, ShouldBeNil)
				So(newrules, ShouldEqual, []string{"/ignoreme/", "*.log", "*.png", "/build/", "/test2/", "/!important.log", "*.txt", "/test/", "/ignoremepls/"})
				newrules, err = gi.AddRulesAt([]string{"/testbad/", "bad/indices/"}, []int{1})
				So(err, ShouldBeNil)
				So(newrules, ShouldEqual, []string{"/ignoreme/", "/testbad/", "*.log", "*.png", "/build/", "/test2/", "/!important.log", "*.txt", "/test/", "/ignoremepls/", "bad/indices/"})
			})

			Convey("And the matcher is successfully updated to correspond with the updated rules", func() {
				paths := []string{"testfile.txt", "testfile2.log", "testfile3.mpg"}
				ignore, keep := gi.Match(paths)
				So(ignore, ShouldResemble, []string{"testfile.txt", "testfile2.log"})
				So(keep, ShouldResemble, []string{"testfile3.mpg"})
			})
		})

		Convey("Rules can be removed", func() {
			newrules, err := gi.RemoveRules([]string{"*.log"})
			So(err, ShouldBeNil)
			So(newrules, ShouldEqual, []string{"/build/", "/!important.log"})

			Convey("And the matcher is successfully updated to correspond with the updated rules", func() {
				paths := []string{"/build/", "test.log"}
				ignore, keep := gi.Match(paths)
				So(ignore, ShouldResemble, []string{"/build/"})
				So(keep, ShouldResemble, []string{"test.log"})
			})
		})
	})
}
