package ruletree

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
)

func TestGlob(t *testing.T) {
	Convey("With a built tree DB", t, func() {
		treeDB := directories.NewRoot("/some/path/", time.Now().Unix())
		directories.AddFile(&treeDB.Directory, "MyDir/a.txt", 1, 2, 3, 4)
		directories.AddFile(&treeDB.Directory, "MyDir/b.tsv", 1, 2, 5, 6)
		directories.AddFile(&treeDB.Directory, "YourDir/c.tsv", 21, 22, 15, 16)
		directories.AddFile(&treeDB.Directory, "OtherDir/a.file", 1, 22, 25, 26)
		directories.AddFile(&treeDB.Directory, "OtherDir/b.file", 1, 2, 35, 36)

		treeDBPathA := createTree(t, treeDB)

		root, err := NewRoot(nil)
		So(err, ShouldBeNil)

		So(root.AddTree(treeDBPathA), ShouldBeNil)

		Convey("You can Glob paths in it", func() {
			So(root.GlobPath("/some/path/*/"), ShouldResemble, []string{
				"/some/path/MyDir/",
				"/some/path/OtherDir/",
				"/some/path/YourDir/",
			})
			So(root.GlobPath("/some/path/???*Dir/"), ShouldResemble, []string{
				"/some/path/OtherDir/",
				"/some/path/YourDir/",
			})
			So(root.GlobPath("/some/path/*/*.tsv"), ShouldResemble, []string{
				"/some/path/MyDir/b.tsv",
				"/some/path/YourDir/c.tsv",
			})
			So(root.GlobPaths("/some/path/*/*.tsv", "/some/path/???*Dir/"), ShouldResemble, []string{
				"/some/path/MyDir/b.tsv",
				"/some/path/OtherDir/",
				"/some/path/YourDir/",
				"/some/path/YourDir/c.tsv",
			})
		})
	})
}
