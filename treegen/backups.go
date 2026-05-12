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

type backupTree struct {
	sizeCount
	backups  sizeCount
	children map[string]tree.Node
}

func newBackupTree() *backupTree {
	return &backupTree{
		children: make(map[string]tree.Node),
	}
}

func (b *backupTree) Children() iter.Seq2[string, tree.Node] {
	return func(yield func(string, tree.Node) bool) {
		for name, child := range b.children {
			if !yield(name, child) {
				return
			}
		}
	}
}

func (b *backupTree) WriteTo(w io.Writer) (int64, error) {
	slw := byteio.StickyLittleEndianWriter{Writer: w}

	slw.WriteUintX(b.size)
	slw.WriteUintX(b.count)

	if b.backups.count > 0 {
		slw.WriteUintX(b.backups.size)
		slw.WriteUintX(b.backups.count)
	}

	return slw.Count, slw.Err
}

func (b *backupTree) AddFile(file string, size uint64) {
	b.count++
	b.size += size

	for part := range iiter.PathParts(file[1:]) {
		c, ok := b.children[part]
		if !ok {
			c = newBackupTree()
			b.children[part] = c
		}

		b = c.(*backupTree)
		b.count++
		b.size += size
	}

	var buf byteio.MemLittleEndian

	buf.WriteUintX(size)

	b.children[filepath.Base(file)] = tree.Leaf(buf)
}

func (b *backupTree) AddLocal(local string, size uint64) {
	for part := range iiter.PathParts(local[1:]) {
		c, ok := b.children[part]
		if !ok {
			c = newBackupTree()
			b.children[part] = c
		}

		b = c.(*backupTree)
	}

	b.backups.count++
	b.backups.size += size
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
		).Execute(ctx),
		backedupScanner,
	).ForEach(func(bf *backedupFile) error {
		remotePath, err := tx(bf.Local)
		if err != nil {
			return fmt.Errorf("error transforming FOFN set path: %w", err)
		} else if !strings.HasPrefix(bf.Remote, remotePath) {
			return ErrInvalidSet
		}

		b.AddFile(filepath.Join(bf.Local, strings.TrimPrefix(bf.Remote, remotePath)), bf.Size)
		b.AddLocal(bf.Local, bf.Size)

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
	c, err := iron.New(context.Background(), env, iron.Option{
		ClientName:    "backup-plans",
		HandshakeFunc: oldHandshake(env),
	})
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
