/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Sky Haines <sh55@sanger.ac.uk>
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
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
	"vimagination.zapto.org/tree"
)

func TestTopLevel(t *testing.T) {
	Convey("Given a tree", t, func() {
		treeDBA := directories.NewRoot("/some/path/", time.Now().Unix())
		directories.AddFile(&treeDBA.Directory, "MyDir/a.txt", 1, 2, 3, 4)
		directories.AddFile(&treeDBA.Directory, "MyDir/b.csv", 1, 2, 5, 6)
		directories.AddFile(&treeDBA.Directory, "YourDir/c.tsv", 21, 22, 15, 16)
		directories.AddFile(&treeDBA.Directory, "OtherDir/a.file", 1, 22, 25, 26)
		directories.AddFile(&treeDBA.Directory, "OtherDir/b.file", 1, 2, 35, 36)

		treeDBPathA := filepath.Join(t.TempDir(), "a.db")

		f, err := os.Create(treeDBPathA)
		So(err, ShouldBeNil)
		So(tree.Serialise(f, treeDBA), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		root, err := NewRoot(nil)
		So(err, ShouldBeNil)

		err = root.AddTree(treeDBPathA)
		So(err, ShouldBeNil)

		Convey("You can check if given children are directories", func() {
			So(root.IsDirectory("/some/path"), ShouldBeTrue)
			So(root.IsDirectory("/some/path/MyDir"), ShouldBeTrue)
			So(root.IsDirectory("/some/path/YourDir/a"), ShouldBeFalse)
			So(root.IsDirectory("/some/path/YourDir/c.tsv"), ShouldBeFalse)
		})
	})
}
