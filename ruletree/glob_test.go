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
