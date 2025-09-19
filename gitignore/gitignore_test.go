package gitignore

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNew(t *testing.T) {
	Convey("Given a gitIgnore filepath, a matcher can be retrieved", t, func() {
		gi, err := New("path/gitignore.txt")
		So(gi, ShouldNotBeNil)
		So(err, ShouldBeNil)

		Convey("Given a matcher, files can be split into ignore/keep", func() {
			var paths []string
			ignore, keep := gi.Match(paths)
			So(ignore, ShouldNotBeNil)
			So(keep, ShouldNotBeNil)
		})
	})
}
