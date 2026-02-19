/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Sendu Bala <sb10@sanger.ac.uk>
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

package ibackup_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
)

func TestFofnStatusReaderGetBackupActivity(t *testing.T) {
	Convey("GetBackupActivity maps fofn status into SetBackupActivity", t, func() {
		baseDir := t.TempDir()
		reader := ibackup.NewFofnStatusReader(baseDir)
		setName := "plan::/lustre/scratch123/humgen/a/b/"
		statusDir := filepath.Join(baseDir, ibackup.SafeName(setName))
		statusPath := filepath.Join(statusDir, "status")

		Convey("Given uploaded and unmodified entries with failed=0, it maps counts and uses mtime", func() {
			err := os.MkdirAll(statusDir, 0o755)
			So(err, ShouldBeNil)

			content := "\"/local/a\"\t\"/remote/a\"\tuploaded\t\"\"\n" +
				"\"/local/b\"\t\"/remote/b\"\tunmodified\t\"\"\n" +
				"SUMMARY\tuploaded=1\treplaced=0\tunmodified=1\tmissing=0\t" +
				"failed=0\tfrozen=0\torphaned=0\twarning=0\thardlink=0\t" +
				"not_processed=0\n"
			err = os.WriteFile(statusPath, []byte(content), 0o600)
			So(err, ShouldBeNil)

			stat, err := os.Stat(statusPath)
			So(err, ShouldBeNil)

			activity, err := reader.GetBackupActivity(setName)
			So(err, ShouldBeNil)
			So(activity, ShouldNotBeNil)
			So(activity.Failures, ShouldEqual, 0)
			So(activity.Uploaded, ShouldEqual, 1)
			So(activity.Unmodified, ShouldEqual, 1)
			So(activity.Replaced, ShouldEqual, 0)
			So(activity.Missing, ShouldEqual, 0)
			So(activity.Frozen, ShouldEqual, 0)
			So(activity.Orphaned, ShouldEqual, 0)
			So(activity.Warning, ShouldEqual, 0)
			So(activity.Hardlink, ShouldEqual, 0)
			So(activity.LastSuccess, ShouldEqual, stat.ModTime())
			So(activity.LastSuccess.IsZero(), ShouldBeFalse)
		})

		Convey("Given failed=2, it returns failures and zero LastSuccess", func() {
			err := os.MkdirAll(statusDir, 0o755)
			So(err, ShouldBeNil)

			content := "\"/local/a\"\t\"/remote/a\"\tfailed\t\"\"\n" +
				"\"/local/b\"\t\"/remote/b\"\tfailed\t\"\"\n" +
				"SUMMARY\tuploaded=3\treplaced=1\tunmodified=4\tmissing=5\t" +
				"failed=2\tfrozen=6\torphaned=7\twarning=8\thardlink=9\t" +
				"not_processed=0\n"
			err = os.WriteFile(statusPath, []byte(content), 0o600)
			So(err, ShouldBeNil)

			activity, err := reader.GetBackupActivity(setName)
			So(err, ShouldBeNil)
			So(activity, ShouldNotBeNil)
			So(activity.Failures, ShouldEqual, 2)
			So(activity.Uploaded, ShouldEqual, 3)
			So(activity.Replaced, ShouldEqual, 1)
			So(activity.Unmodified, ShouldEqual, 4)
			So(activity.Missing, ShouldEqual, 5)
			So(activity.Frozen, ShouldEqual, 6)
			So(activity.Orphaned, ShouldEqual, 7)
			So(activity.Warning, ShouldEqual, 8)
			So(activity.Hardlink, ShouldEqual, 9)
			So(activity.LastSuccess, ShouldEqual, time.Time{})
		})

		Convey("Given no status file, it returns nil and no error", func() {
			activity, err := reader.GetBackupActivity(setName)
			So(err, ShouldBeNil)
			So(activity, ShouldBeNil)
		})

		Convey("Given all zeros and only not_processed, it ignores not_processed and uses mtime", func() {
			err := os.MkdirAll(statusDir, 0o755)
			So(err, ShouldBeNil)

			content := "SUMMARY\tuploaded=0\treplaced=0\tunmodified=0\tmissing=0\t" +
				"failed=0\tfrozen=0\torphaned=0\twarning=0\thardlink=0\t" +
				"not_processed=11\n"
			err = os.WriteFile(statusPath, []byte(content), 0o600)
			So(err, ShouldBeNil)

			stat, err := os.Stat(statusPath)
			So(err, ShouldBeNil)

			activity, err := reader.GetBackupActivity(setName)
			So(err, ShouldBeNil)
			So(activity, ShouldNotBeNil)
			So(activity.Failures, ShouldEqual, 0)
			So(activity.Uploaded, ShouldEqual, 0)
			So(activity.Replaced, ShouldEqual, 0)
			So(activity.Unmodified, ShouldEqual, 0)
			So(activity.Missing, ShouldEqual, 0)
			So(activity.Frozen, ShouldEqual, 0)
			So(activity.Orphaned, ShouldEqual, 0)
			So(activity.Warning, ShouldEqual, 0)
			So(activity.Hardlink, ShouldEqual, 0)
			So(activity.LastSuccess, ShouldEqual, stat.ModTime())
		})

		Convey("Given malformed status, it returns a parse error", func() {
			err := os.MkdirAll(statusDir, 0o755)
			So(err, ShouldBeNil)

			err = os.WriteFile(statusPath, []byte("not a valid status file\n"), 0o600)
			So(err, ShouldBeNil)

			activity, err := reader.GetBackupActivity(setName)
			So(activity, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}
