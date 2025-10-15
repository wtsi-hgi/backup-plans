package directories

import (
	"fmt"
	"io"
	"iter"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wtsi-hgi/backup-plans/treegen"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"vimagination.zapto.org/tree"
)

const dirSize = 4096

type child interface {
	tree.Node
	writeTo(w io.Writer)
}

type Root struct {
	Directory
}

// Directory represents the stat information for a directory and its children.
type Directory struct {
	children map[string]child
	Path     summary.DirectoryPath
	treegen.Directory
	parent *Directory
}

// NewRoot creates a new Directory root with the specified time as the atime,
// mtime, and ctime.
func NewRoot(path string, refTime int64) *Root {
	return &Root{
		Directory: Directory{
			children: make(map[string]child),
			Path:     summary.DirectoryPath{Name: path},
			Directory: treegen.Directory{
				MTime:  refTime,
				Users:  make(treegen.IDMeta),
				Groups: make(treegen.IDMeta),
			},
		},
	}
}

func (r *Root) Children() iter.Seq2[string, tree.Node] {
	return func(yield func(string, tree.Node) bool) {
		yield(r.Path.Name, &r.Directory)
	}
}

func (r *Root) WriteTo(_ io.Writer) (int64, error) {
	return 0, nil
}

// AddDirectory either creates and returns a new directory in the direcory or
// returns an existing one.
func (d *Directory) AddDirectory(name string) *Directory {
	if name == "." {
		return d
	} else if !strings.HasSuffix(name, "/") {
		name += "/"
	}

	if c, ok := d.children[name]; ok {
		if cd, ok := c.(*Directory); ok {
			return cd
		}

		return nil
	}

	c := &Directory{
		children: make(map[string]child),
		Path:     summary.DirectoryPath{Parent: &d.Path, Name: name},
		Directory: treegen.Directory{
			MTime:  d.MTime,
			UID:    d.UID,
			GID:    d.GID,
			Users:  make(treegen.IDMeta),
			Groups: make(treegen.IDMeta),
		},
		parent: d,
	}

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

	f := &File{
		Path: summary.DirectoryPath{
			Parent: &d.Path,
			Name:   name,
		},
		Type:   'f',
		parent: d,
	}

	d.children[name] = f

	return f
}

// WriteTo writes the stats data for the directory.
func (d *Directory) writeTo(w io.Writer) {
	path := string(d.Path.AppendTo(nil))
	fmt.Fprintf(w, "%q\t%d\t%d\t%d\t%d\t%d\t%d\t%c\t%d\t1\t1\t%d\n", // nolint:errcheck
		path, 4096, d.UID, d.GID, d.MTime, d.MTime, d.MTime, 'd', 0, 4096)

	keys := slices.Collect(maps.Keys(d.children))

	slices.Sort(keys)

	for _, k := range keys {
		d.children[k].writeTo(w)
	}
}

// AsReader returns a ReadCloser that will output a stats file as read by the
// stats package.
func (d *Directory) AsReader() io.ReadCloser {
	pr, pw := io.Pipe()

	go func() {
		d.writeTo(pw) // nolint:errcheck
		pw.Close()    // nolint:errcheck
	}()

	return pr
}

func (d *Directory) SetMeta(uid, gid uint32, mtime int64) *Directory {
	d.UID = uid
	d.GID = gid
	d.MTime = mtime

	return d
}

func (d *Directory) Children() iter.Seq2[string, tree.Node] {
	return func(yield func(string, tree.Node) bool) {
		keys := slices.Collect(maps.Keys(d.children))

		slices.Sort(keys)

		for _, name := range keys {
			if !yield(name, d.children[name]) {
				return
			}
		}
	}
}

func (d *Directory) addFileData(uid, gid uint32, mtime, size int64) {
	if d.parent != nil {
		d.parent.addFileData(uid, gid, mtime, size)
	}

	d.Add(uid, gid, mtime, size)
}

// File represents a pseudo-file entry.
type File struct {
	treegen.File
	Path   summary.DirectoryPath
	Inode  uint64
	Type   byte
	parent *Directory
}

// writeTo writes the stats data for a file entry.
func (f *File) writeTo(w io.Writer) {
	path := string(f.Path.AppendTo(nil))
	fmt.Fprintf(w, "%q\t%d\t%d\t%d\t%d\t%d\t%d\t%c\t%d\t1\t1\t%d\n", // nolint:errcheck
		path, f.Size, f.UID, f.GID, f.MTime, f.MTime, f.MTime, f.Type, f.Inode, f.Size)
}

func (f *File) WriteTo(w io.Writer) (int64, error) {
	f.parent.addFileData(f.UID, f.GID, f.MTime, f.Size)

	return f.File.WriteTo(w)
}

// AddFile adds file data to a directory, creating the directory in the tree if
// necessary.
func AddFile(d *Directory, path string, uid, gid uint32, size, mtime int64) *File {
	for part := range strings.SplitSeq(filepath.Dir(path), "/") {
		d = d.AddDirectory(part)
	}

	file := d.AddFile(filepath.Base(path))
	file.UID = uid
	file.GID = gid
	file.Size = size
	file.MTime = mtime

	return file
}
