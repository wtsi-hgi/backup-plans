package backuptree

import (
	"cmp"
	"io"
	"iter"
	"path/filepath"
	"strings"

	iiter "github.com/wtsi-hgi/backup-plans/internal/iter"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

// BackupTree contains information about files that have been automatically
// backed up.
//
// Child nodes are either:
//
//	A directory, of type *BackupTree
//	A file, of type *tree.Leaf, which contains the encoded size.
//	A special collection node, identified by the empty string path, of type
//	  *BackupTree.
//
// The special collection is similar to a normal directory, except it only
// contains files that belong to that collection. All file nodes have no data.
//
// The data for a directory is the total size of the files held within and a
// count of the number of files.
//
// The data for a file is the size of that file, unless it is recorded in a
// special collection Node, in which case it has no data.
type BackupTree struct {
	backupSize, backupCount   uint64
	archiveSize, archiveCount uint64
	children                  map[string]tree.Node
	collection                *BackupTree
}

// New creates a new, empty BackupTree ready to add files to.
func New() *BackupTree {
	return &BackupTree{
		children: make(map[string]tree.Node),
	}
}

// Children is required to implement the tree.Node interface.
func (b *BackupTree) Children() iter.Seq2[string, tree.Node] {
	return func(yield func(string, tree.Node) bool) {
		for name, child := range iiter.SortedMap(b.children) {
			if name == "" {
				continue
			}

			if !yield(name, child) {
				return
			}
		}
	}
}

// WriteTo implements the io.WriterTo interface required for the tree.Node
// interface.
func (b *BackupTree) WriteTo(w io.Writer) (int64, error) {
	slw := byteio.StickyLittleEndianWriter{Writer: w}

	slw.WriteUintX(b.backupSize)
	slw.WriteUintX(b.backupCount)
	slw.WriteUintX(b.archiveSize)
	slw.WriteUintX(b.archiveCount)

	if collection, ok := b.children[""]; ok {
		if err := tree.Serialise(&slw, collection); err != nil {
			return slw.Count, cmp.Or(slw.Err, err)
		}
	}

	return slw.Count, slw.Err
}

var (
	hasLocal = tree.Leaf{1}
	noLocal  tree.Leaf
)

// AddFileToCollection adds a file to the directory tree and to the collection
// specified.
//
// The collection should take the form of a local path, and the given path
// should be the rest of the file path.
func (b *BackupTree) AddFileToCollection(collection, path string, size uint64, local bool) {
	b = b.navigateTo(collection, size, local)

	b.addFileToDir(path, size, local)

	b = b.getChildDir("").navigateTo(path, size, local)

	b.addSize(size, local)

	if local {
		b.children[filepath.Base(path)] = &hasLocal
	} else {
		b.children[filepath.Base(path)] = &noLocal
	}
}

func (b *BackupTree) navigateTo(path string, size uint64, local bool) *BackupTree {
	for part := range iiter.PathParts(strings.TrimPrefix(path, "/")) {
		b.addSize(size, local)

		b = b.getChildDir(part)
	}

	return b
}

func (b *BackupTree) addSize(size uint64, local bool) {
	if local {
		b.backupCount++
		b.backupSize += size
	} else {
		b.archiveCount++
		b.archiveSize += size
	}
}

func (b *BackupTree) getChildDir(name string) *BackupTree {
	c, ok := b.children[name]
	if !ok {
		c = New()
		b.children[name] = c
	}

	return c.(*BackupTree)
}

func (b *BackupTree) addFileToDir(path string, size uint64, local bool) {
	b = b.navigateTo(path, size, local)

	b.addSize(size, local)

	var buf byteio.MemLittleEndian

	buf.WriteUintX(size)
	buf.WriteBool(local)

	b.children[filepath.Base(path)] = tree.Leaf(buf)
}

// Generate builds a BackupTree from the given map, which should look like:
//
// map[collection name: string]map[file path: string]size: uint64
//
// There must be no duplications of collection + file path.
func Generate(collections map[string]map[string]uint64) *BackupTree {
	bt := New()

	for collection, files := range collections {
		for path, size := range files {
			bt.AddFileToCollection(collection, path, size, false)
		}
	}

	return bt
}
