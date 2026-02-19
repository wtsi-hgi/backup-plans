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

package ibackup_test

import (
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/ibackup/fofn"
)

func TestSafeName(t *testing.T) {
	Convey("SafeName converts set names to filesystem-safe names", t, func() {
		So(ibackup.SafeName("plan::/lustre/scratch123/humgen/a/b/"), ShouldEqual,
			"／lustre／scratch123／humgen／a／b／")
		So(ibackup.SafeName("plan::/a/"), ShouldEqual, "／a／")
		So(ibackup.SafeName("plan::/"), ShouldEqual, "／")
		So(ibackup.SafeName("/lustre/scratch/"), ShouldEqual, "／lustre／scratch／")
		So(ibackup.SafeName(""), ShouldEqual, "")
	})
}

func TestFofnDirWriterWrite(t *testing.T) {
	Convey("Write creates fofn and config files in a safe subdirectory", t, func() {
		baseDir := t.TempDir()
		writer := ibackup.NewFofnDirWriter(baseDir)
		setName := "plan::/lustre/scratch123/humgen/a/b/"

		metadata := map[string]string{
			"requestor": "userA",
			"review":    "2027-01-01",
			"remove":    "2027-06-01",
		}

		files := []string{"/tmp/file1", "/tmp/file2", "/tmp/file3"}
		seq := func(yield func(string) bool) {
			for _, file := range files {
				if !yield(file) {
					return
				}
			}
		}

		wrote, err := writer.Write(setName, "humgen", seq, 7, metadata)
		So(err, ShouldBeNil)
		So(wrote, ShouldBeTrue)

		subDir := filepath.Join(baseDir, ibackup.SafeName(setName))
		info, err := os.Stat(subDir)
		So(err, ShouldBeNil)
		So(info.IsDir(), ShouldBeTrue)

		fofnBytes, err := os.ReadFile(filepath.Join(subDir, "fofn"))
		So(err, ShouldBeNil)
		So(string(fofnBytes), ShouldEqual, "/tmp/file1\x00/tmp/file2\x00/tmp/file3\x00")

		cfg, err := fofn.ReadConfig(subDir)
		So(err, ShouldBeNil)
		So(cfg.Transformer, ShouldEqual, "humgen")
		So(cfg.Freeze, ShouldBeFalse)
		So(cfg.Metadata["requestor"], ShouldEqual, "userA")
		So(cfg.Metadata["review"], ShouldEqual, "2027-01-01")
		So(cfg.Metadata["remove"], ShouldEqual, "2027-06-01")
	})

	Convey("Write sets freeze true in config when frequency is 0", t, func() {
		baseDir := t.TempDir()
		writer := ibackup.NewFofnDirWriter(baseDir)
		setName := "plan::/a/"

		metadata := map[string]string{
			"requestor": "userB",
			"review":    "2028-01-01",
			"remove":    "2028-06-01",
		}

		seq := func(yield func(string) bool) {
			yield("/some/path")
		}

		wrote, err := writer.Write(setName, "humgen", seq, 0, metadata)
		So(err, ShouldBeNil)
		So(wrote, ShouldBeTrue)

		subDir := filepath.Join(baseDir, ibackup.SafeName(setName))
		cfg, err := fofn.ReadConfig(subDir)
		So(err, ShouldBeNil)
		So(cfg.Freeze, ShouldBeTrue)
	})

	Convey("Write returns false and creates no subdirectory for an empty iterator", t, func() {
		baseDir := t.TempDir()
		writer := ibackup.NewFofnDirWriter(baseDir)
		setName := "plan::/empty/"

		metadata := map[string]string{
			"requestor": "userC",
			"review":    "2029-01-01",
			"remove":    "2029-06-01",
		}

		emptySeq := iter.Seq[string](func(_ func(string) bool) {})

		wrote, err := writer.Write(setName, "humgen", emptySeq, 7, metadata)
		So(err, ShouldBeNil)
		So(wrote, ShouldBeFalse)

		_, err = os.Stat(filepath.Join(baseDir, ibackup.SafeName(setName)))
		So(os.IsNotExist(err), ShouldBeTrue)
	})

	Convey("Write writes a single path followed by one trailing null byte", t, func() {
		baseDir := t.TempDir()
		writer := ibackup.NewFofnDirWriter(baseDir)
		setName := "plan::/single/"

		metadata := map[string]string{
			"requestor": "userD",
			"review":    "2030-01-01",
			"remove":    "2030-06-01",
		}

		seq := func(yield func(string) bool) {
			yield("/a/b/c.txt")
		}

		wrote, err := writer.Write(setName, "humgen", seq, 7, metadata)
		So(err, ShouldBeNil)
		So(wrote, ShouldBeTrue)

		subDir := filepath.Join(baseDir, ibackup.SafeName(setName))
		fofnBytes, err := os.ReadFile(filepath.Join(subDir, "fofn"))
		So(err, ShouldBeNil)
		So(string(fofnBytes), ShouldEqual, "/a/b/c.txt\x00")
	})

	Convey("Write streams 1,000,000 paths without large heap growth", t, func() {
		baseDir := t.TempDir()
		writer := ibackup.NewFofnDirWriter(baseDir)
		setName := "plan::/memory/"

		metadata := map[string]string{
			"requestor": "userE",
			"review":    "2031-01-01",
			"remove":    "2031-06-01",
		}

		const (
			nPaths   = 1_000_000
			pathSize = 100
		)

		seq := iter.Seq[string](func(yield func(string) bool) {
			for i := range nPaths {
				path := fmt.Sprintf("/%098dX", i)
				if !yield(path[:pathSize]) {
					return
				}
			}
		})

		runtime.GC()

		var before runtime.MemStats

		runtime.ReadMemStats(&before)

		wrote, err := writer.Write(setName, "humgen", seq, 7, metadata)
		So(err, ShouldBeNil)
		So(wrote, ShouldBeTrue)

		runtime.GC()

		var after runtime.MemStats

		runtime.ReadMemStats(&after)

		var growth uint64

		if after.HeapInuse > before.HeapInuse {
			growth = after.HeapInuse - before.HeapInuse
		}

		So(growth, ShouldBeLessThan, 10*1024*1024)
	})
}

func TestFofnDirWriterUpdateConfig(t *testing.T) {
	Convey("UpdateConfig rewrites only config for existing subdirectories", t,
		func() {
			baseDir := t.TempDir()
			writer := ibackup.NewFofnDirWriter(baseDir)
			setName := "plan::/claim/"

			subDir := filepath.Join(baseDir, ibackup.SafeName(setName))
			err := os.MkdirAll(subDir, 0o755)
			So(err, ShouldBeNil)

			fofnPath := filepath.Join(subDir, "fofn")
			err = os.WriteFile(fofnPath, []byte("/path/one\x00/path/two\x00"), 0o600)
			So(err, ShouldBeNil)

			initialMetadata := map[string]string{
				"requestor": "userA",
				"review":    "2027-01-01",
				"remove":    "2027-06-01",
			}

			err = fofn.WriteConfig(subDir, fofn.SubDirConfig{
				Transformer: "humgen",
				Freeze:      false,
				Metadata:    initialMetadata,
			})
			So(err, ShouldBeNil)

			beforeBytes, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)

			beforeInfo, err := os.Stat(fofnPath)
			So(err, ShouldBeNil)

			updatedMetadata := map[string]string{
				"requestor": "userB",
				"review":    "2027-01-01",
				"remove":    "2027-06-01",
			}

			err = writer.UpdateConfig(setName, "humgen", false, updatedMetadata)
			So(err, ShouldBeNil)

			cfg, err := fofn.ReadConfig(subDir)
			So(err, ShouldBeNil)
			So(cfg.Metadata["requestor"], ShouldEqual, "userB")

			afterBytes, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(afterBytes), ShouldEqual, string(beforeBytes))

			afterInfo, err := os.Stat(fofnPath)
			So(err, ShouldBeNil)
			So(afterInfo.ModTime(), ShouldEqual, beforeInfo.ModTime())
		})

	Convey("UpdateConfig returns an error when subdirectory does not exist", t,
		func() {
			baseDir := t.TempDir()
			writer := ibackup.NewFofnDirWriter(baseDir)

			err := writer.UpdateConfig("plan::/missing/", "humgen", false,
				map[string]string{"requestor": "userB"})
			So(err, ShouldNotBeNil)
		})
}

func TestFofnDirWriterWriteFrequencyGating(t *testing.T) {
	Convey("Write applies frequency gating to existing fofn files", t, func() {
		baseDir := t.TempDir()
		writer := ibackup.NewFofnDirWriter(baseDir)
		setName := "plan::/gating/"

		metadata := map[string]string{
			"requestor": "userA",
			"review":    "2027-01-01",
			"remove":    "2027-06-01",
		}

		subDir := filepath.Join(baseDir, ibackup.SafeName(setName))
		fofnPath := filepath.Join(subDir, "fofn")

		newSeq := iter.Seq[string](func(yield func(string) bool) {
			yield("/new/path")
		})

		Convey("Given no existing fofn and frequency 7, Write writes and returns true", func() {
			wrote, err := writer.Write(setName, "humgen", newSeq, 7, metadata)
			So(err, ShouldBeNil)
			So(wrote, ShouldBeTrue)

			content, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "/new/path\x00")
		})

		Convey("Given existing fofn mtime 1 hour ago and frequency 1, Write skips", func() {
			createExistingFofn(t, fofnPath, time.Now().Add(-1*time.Hour))

			wrote, err := writer.Write(setName, "humgen", newSeq, 1, metadata)
			So(err, ShouldBeNil)
			So(wrote, ShouldBeFalse)

			content, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "/existing\x00")
		})

		Convey("Given existing fofn mtime 13 hours ago and frequency 1, Write rewrites", func() {
			createExistingFofn(t, fofnPath, time.Now().Add(-13*time.Hour))

			wrote, err := writer.Write(setName, "humgen", newSeq, 1, metadata)
			So(err, ShouldBeNil)
			So(wrote, ShouldBeTrue)

			content, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "/new/path\x00")
		})

		Convey("Given existing fofn mtime 36 hours ago and frequency 2, Write skips", func() {
			createExistingFofn(t, fofnPath, time.Now().Add(-36*time.Hour+500*time.Millisecond))

			wrote, err := writer.Write(setName, "humgen", newSeq, 2, metadata)
			So(err, ShouldBeNil)
			So(wrote, ShouldBeFalse)

			content, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "/existing\x00")
		})

		Convey("Given existing fofn mtime 37 hours ago and frequency 2, Write rewrites", func() {
			createExistingFofn(t, fofnPath, time.Now().Add(-37*time.Hour))

			wrote, err := writer.Write(setName, "humgen", newSeq, 2, metadata)
			So(err, ShouldBeNil)
			So(wrote, ShouldBeTrue)

			content, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "/new/path\x00")
		})

		Convey("Given existing fofn and frequency 0, Write skips", func() {
			createExistingFofn(t, fofnPath, time.Now().Add(-100*time.Hour))

			wrote, err := writer.Write(setName, "humgen", newSeq, 0, metadata)
			So(err, ShouldBeNil)
			So(wrote, ShouldBeFalse)

			content, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "/existing\x00")
		})

		Convey("Given no existing fofn and frequency 0, Write writes and returns true", func() {
			wrote, err := writer.Write(setName, "humgen", newSeq, 0, metadata)
			So(err, ShouldBeNil)
			So(wrote, ShouldBeTrue)

			content, err := os.ReadFile(fofnPath)
			So(err, ShouldBeNil)
			So(string(content), ShouldEqual, "/new/path\x00")
		})
	})
}

func createExistingFofn(t *testing.T, fofnPath string, mtime time.Time) {
	t.Helper()

	err := os.MkdirAll(filepath.Dir(fofnPath), 0o755)
	So(err, ShouldBeNil)

	err = os.WriteFile(fofnPath, []byte("/existing\x00"), 0o600)
	So(err, ShouldBeNil)

	err = os.Chtimes(fofnPath, mtime, mtime)
	So(err, ShouldBeNil)
}
