package treegen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"path"
	"path/filepath"
	"strings"

	"github.com/kuleuven/iron"
	"github.com/kuleuven/iron/api"
	"github.com/kuleuven/iron/msg"
	iiter "github.com/wtsi-hgi/backup-plans/internal/iter"
	"github.com/wtsi-hgi/ibackup/transformer"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

var (
	ErrInvalidJSON = errors.New("invalid JSON response")
	ErrInvalidSet  = errors.New("invalid set for backup")
)

type sizeCount struct {
	size, count uint64
}

func (c *sizeCount) WriteTo(w io.Writer) (int64, error) {
	slw := byteio.StickyLittleEndianWriter{Writer: w}

	slw.WriteUintX(c.size)
	slw.WriteUintX(c.count)

	return slw.Count, slw.Err
}

type fileTree struct {
	sizeCount
	children map[string]*fileTree
	files    map[string]tree.Leaf // size
}

func newFileTree() *fileTree {
	return &fileTree{
		children: make(map[string]*fileTree),
		files:    make(map[string]tree.Leaf),
	}
}

func (c *fileTree) Children() iter.Seq2[string, tree.Node] { //nolint:gocognit
	return func(yield func(string, tree.Node) bool) {
		for name, child := range c.children {
			if !yield(name, child) {
				return
			}
		}

		for name, child := range c.files {
			if !yield(name, child) {
				return
			}
		}
	}
}

func (c *fileTree) AddFile(file string, size uint64) {
	c.count++
	c.size += size

	for part := range iiter.PathParts(file[1:]) {
		g, ok := c.children[part]
		if !ok {
			g = newFileTree()
			c.children[part] = g
		}

		c = g
		c.count++
		c.size += size
	}

	var buf byteio.MemLittleEndian

	buf.WriteUintX(size)

	c.files[filepath.Base(file)] = tree.Leaf(buf)
}

type backupTree struct {
	sizeCount
	children map[string]*backupTree
	files    *fileTree
}

func newBackupTree() *backupTree {
	return &backupTree{
		children: make(map[string]*backupTree),
	}
}

func (b *backupTree) Children() iter.Seq2[string, tree.Node] {
	return func(yield func(string, tree.Node) bool) {
		for name, child := range b.children {
			if !yield(name, child) {
				return
			}
		}

		if b.files != nil {
			yield("/", b.files)
		}
	}
}

func (b *backupTree) AddFile(set, file string, size uint64) {
	b.count++
	b.size += size

	for part := range iiter.PathParts(set[1:]) {
		c, ok := b.children[part]
		if !ok {
			c = newBackupTree()
			b.children[part] = c
		}

		b = c
		b.count++
		b.size += size
	}

	if b.files == nil {
		b.files = newFileTree()
	}

	b.files.AddFile(file, size)
}

func (b *backupTree) AddCollection(a *api.API, collection string, tx transformer.PathTransformer) error {
	ctx, cFn := context.WithCancel(context.Background())

	defer cFn()

	return iiter.Rows(
		a.Query(
			msg.ICAT_COLUMN_COLL_NAME,
			msg.ICAT_COLUMN_DATA_NAME,
			msg.ICAT_COLUMN_META_DATA_ATTR_VALUE,
			msg.ICAT_COLUMN_DATA_SIZE,
		).With(
			api.Like(msg.ICAT_COLUMN_COLL_NAME, strings.TrimSuffix(collection, "/")+"/%"),
			api.Equal(msg.ICAT_COLUMN_META_DATA_ATTR_NAME, "ibackup:fofn:set"),
		).Execute(ctx), backedupScanner).ForEach(func(bf *backedupFile) error {
		remotePath, err := tx(bf.Local)
		if err != nil {
			return fmt.Errorf("error transforming FOFN set path: %w", err)
		} else if !strings.HasPrefix(bf.Remote, remotePath) {
			return ErrInvalidSet
		}

		b.AddFile(bf.Local, strings.TrimPrefix(bf.Remote, remotePath), bf.Size)

		return nil
	})
}

type backedupFile struct {
	Remote string
	Local  string
	Size   uint64
}

func backedupScanner(s iiter.Scanner) (*backedupFile, error) {
	var (
		b                               backedupFile
		collectionName, dataObject, set string
	)

	if err := s.Scan(&collectionName, &dataObject, &set, &b.Size); err != nil {
		return nil, err
	}

	if !strings.HasPrefix(set, "plan::/") {
		return nil, ErrInvalidSet
	}

	b.Local = strings.TrimPrefix(set, "plan::")
	b.Remote = path.Join(collectionName, dataObject)

	return &b, nil
}

func BackupTree(env iron.Env, collections map[string]transformer.PathTransformer) (tree.Node, error) {
	c, err := iron.New(context.Background(), env, iron.Option{ClientName: "backup-plans"})
	if err != nil {
		return nil, err
	}

	defer c.Close()

	return processCollections(c.API, collections)
}

func processCollections(a *api.API, collections map[string]transformer.PathTransformer) (tree.Node, error) {
	t := newBackupTree()

	for collection, tx := range collections {
		if err := t.AddCollection(a, collection, tx); err != nil {
			return nil, err
		}
	}

	return t, nil
}
