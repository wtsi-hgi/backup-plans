package treegen

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/wtsi-hgi/wrstat-ui/stats"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

func TestTimeTree(t *testing.T) {
	Convey("With a summarised time tree", t, func() {
		f := NewRoot("/", 12345)

		f.AddDirectory("opt").SetMeta(99, 98, 1).AddDirectory("userDir").SetMeta(1, 1, 98765)
		AddFile(f, "opt/userDir/file1.txt", 1, 1, 9, 0, 98766)
		AddFile(f, "opt/userDir/file2.txt", 1, 2, 8, 0, 98767)
		AddFile(f, "opt/subDir/subsubDir/file3.txt", 1, 2, 7, 0, 98000)

		f.AddDirectory("opt").AddDirectory("other").SetMeta(2, 1, 12349)
		AddFile(f, "opt/other/someDir/someFile", 2, 1, 6, 0, 12346)
		AddFile(f, "opt/other/someDir/someFile", 2, 1, 5, 0, 12346)

		p := stats.NewStatsParser(f.AsReader())
		s := summary.NewSummariser(p)

		var treeDB bytes.Buffer

		s.AddDirectoryOperation(NewTree(&treeDB))

		So(s.Summarise(), ShouldBeNil)

		tr, err := tree.OpenMem(treeDB.Bytes())
		So(err, ShouldBeNil)

		tr, err = tr.Child("/")
		So(err, ShouldBeNil)

		Convey("You can read a root summary", func() {
			r := bytes.NewReader(tr.Data())

			uid, gid, userSummary, groupSummary := readSummary(r)
			So(uid, ShouldEqual, 0)
			So(gid, ShouldEqual, 0)
			So(userSummary, ShouldResemble, []IDData{
				{1, &Meta{MTime: 98767, Files: 3, Bytes: 24}},
				{2, &Meta{MTime: 12346, Files: 1, Bytes: 5}},
			})
			So(groupSummary, ShouldResemble, []IDData{
				{1, &Meta{MTime: 98766, Files: 2, Bytes: 14}},
				{2, &Meta{MTime: 98767, Files: 2, Bytes: 15}},
			})

			So(slices.Sorted(maps.Keys(maps.Collect(tr.Children()))), ShouldResemble, []string{
				"opt/",
			})
		})

		Convey("You can read a subdirectory summary", func() {
			tr, err = tr.Child("opt/")
			So(err, ShouldBeNil)

			r := bytes.NewReader(tr.Data())

			uid, gid, userSummary, groupSummary := readSummary(r)
			So(uid, ShouldEqual, 99)
			So(gid, ShouldEqual, 98)
			So(userSummary, ShouldResemble, []IDData{
				{1, &Meta{MTime: 98767, Files: 3, Bytes: 24}},
				{2, &Meta{MTime: 12346, Files: 1, Bytes: 5}},
			})
			So(groupSummary, ShouldResemble, []IDData{
				{1, &Meta{MTime: 98766, Files: 2, Bytes: 14}},
				{2, &Meta{MTime: 98767, Files: 2, Bytes: 15}},
			})

			So(slices.Sorted(maps.Keys(maps.Collect(tr.Children()))), ShouldResemble, []string{
				"other/", "subDir/", "userDir/",
			})
		})

		Convey("You can read the ownership and mtime for a file", func() {
			tr, err = tr.Child("opt/")
			So(err, ShouldBeNil)

			tr, err = tr.Child("userDir/")
			So(err, ShouldBeNil)

			tr, err = tr.Child("file1.txt")
			So(err, ShouldBeNil)

			r := bytes.NewReader(tr.Data())

			rootUID, rootGID, rootMtime, rootBytes := readMeta(r)
			So(rootUID, ShouldEqual, 1)
			So(rootGID, ShouldEqual, 1)
			So(rootMtime, ShouldEqual, 98766)
			So(rootBytes, ShouldEqual, 9)
		})
	})
}

func readMeta(r io.Reader) (uint32, uint32, int64, uint64) {
	lr := byteio.StickyLittleEndianReader{Reader: r}

	return uint32(lr.ReadUintX()), uint32(lr.ReadUintX()), int64(lr.ReadUintX()), lr.ReadUintX()
}

func readSummary(r io.Reader) (uint32, uint32, []IDData, []IDData) {
	lr := byteio.StickyLittleEndianReader{Reader: r}

	uid := lr.ReadUintX()
	gid := lr.ReadUintX()

	lr.ReadUint8()
	lr.ReadUint8()

	return uint32(uid), uint32(gid), readArray(&lr), readArray(&lr)
}

func readArray(lr *byteio.StickyLittleEndianReader) []IDData {
	idts := make([]IDData, lr.ReadUintX())

	for n := range idts {
		idts[n].ID = uint32(lr.ReadUintX())
		idts[n].Meta = new(Meta)
		idts[n].MTime = lr.ReadUintX()
		idts[n].Files = lr.ReadUintX()
		idts[n].Bytes = lr.ReadUintX()
	}

	return idts
}

// Directory represents the stat information for a directory and its children.
type Directory struct {
	children map[string]io.WriterTo
	File
}

// NewRoot creates a new Directory root with the specified time as the atime,
// mtime, and ctime.
func NewRoot(path string, refTime int64) *Directory {
	return &Directory{
		children: make(map[string]io.WriterTo),
		File: File{
			Path:  path,
			Size:  4096,
			ATime: refTime,
			MTime: refTime,
			CTime: refTime,
			Type:  'd',
		},
	}
}

// AddDirectory either creates and returns a new directory in the direcory or
// returns an existing one.
func (d *Directory) AddDirectory(name string) *Directory {
	if name == "." {
		return d
	}

	if c, ok := d.children[name]; ok {
		if cd, ok := c.(*Directory); ok {
			return cd
		}

		return nil
	}

	c := &Directory{
		children: make(map[string]io.WriterTo),
		File:     d.File,
	}

	c.File.Path += name + "/"
	d.children[name] = c

	return c
}

// AddFile either creates and returns a new file in the direcory or returns an
// existing one.
func (d *Directory) AddFile(name string) *File {
	if c, ok := d.children[name]; ok {
		if cf, ok := c.(*File); ok {
			return cf
		}

		return nil
	}

	f := d.File

	d.children[name] = &f
	f.Path += name
	f.Size = 0
	f.Type = 'f'

	return &f
}

// WriteTo writes the stats data for the directory.
func (d *Directory) WriteTo(w io.Writer) (int64, error) {
	n, err := d.File.WriteTo(w)
	if err != nil {
		return n, err
	}

	keys := slices.Collect(maps.Keys(d.children))
	sort.Strings(keys)

	for _, k := range keys {
		m, err := d.children[k].WriteTo(w)

		n += m

		if err != nil {
			return n, err
		}
	}

	return n, nil
}

// AsReader returns a ReadCloser that will output a stats file as read by the
// stats package.
func (d *Directory) AsReader() io.ReadCloser {
	pr, pw := io.Pipe()

	go func() {
		d.WriteTo(pw) //nolint:errcheck
		pw.Close()
	}()

	return pr
}

func (d *Directory) SetMeta(uid, gid uint32, mtime int64) *Directory {
	d.UID = uid
	d.GID = gid
	d.MTime = mtime

	return d
}

// File represents a pseudo-file entry.
type File struct {
	Path                string
	Size                int64
	ATime, MTime, CTime int64
	UID, GID            uint32
	Inode               uint64
	Type                byte
}

// WriteTo writes the stats data for a file entry.
func (f *File) WriteTo(w io.Writer) (int64, error) {
	n, err := fmt.Fprintf(w, "%q\t%d\t%d\t%d\t%d\t%d\t%d\t%c\t%d\t1\t1\t%d\n",
		f.Path, f.Size, f.UID, f.GID, f.ATime, f.MTime, f.CTime, f.Type, f.Inode, f.Size)

	return int64(n), err
}

// AddFile adds file data to a directory, creating the directory in the tree if
// necessary.
func AddFile(d *Directory, path string, uid, gid uint32, size, atime, mtime int64) *File {
	for part := range strings.SplitSeq(filepath.Dir(path), "/") {
		d = d.AddDirectory(part)
	}

	file := d.AddFile(filepath.Base(path))
	file.UID = uid
	file.GID = gid
	file.Size = size
	file.ATime = atime
	file.MTime = mtime

	return file
}
