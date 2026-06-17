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
	"path"
	"path/filepath"
	"strings"

	"github.com/kuleuven/iron"
	"github.com/kuleuven/iron/api"
	"github.com/kuleuven/iron/msg"
	"github.com/wtsi-hgi/backup-plans/internal/backuptree"
	iiter "github.com/wtsi-hgi/backup-plans/internal/iter"
	"github.com/wtsi-hgi/backup-plans/internal/memtree"
	"github.com/wtsi-hgi/ibackup/transformer"
	"vimagination.zapto.org/tree"
)

var (
	ErrInvalidJSON = errors.New("invalid JSON response")
	ErrInvalidSet  = errors.New("invalid set for backup")
)

type backupTree struct {
	*backuptree.BackupTree
}

func newBackupTree() *backupTree {
	return &backupTree{backuptree.New()}
}

// AddCollection searches iRODS for files backed up by Backup Plans with the
// given remove collection.
//
// The transformer is used to reverse engineer the local path from metadata on
// the files.
//
// The fileExists function should be a function that takes a local path and
// returns true if the file exists locally.
func (b *backupTree) AddCollection(a *api.API, collection string,
	tx transformer.PathTransformer, fileExists func(string) bool) error {
	ctx, cFn := context.WithCancel(context.Background())

	defer cFn()

	return iiter.Rows(
		a.Query(
			msg.ICAT_COLUMN_COLL_NAME, msg.ICAT_COLUMN_DATA_NAME,
			msg.ICAT_COLUMN_META_DATA_ATTR_VALUE, msg.ICAT_COLUMN_DATA_SIZE,
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

		remoteSuffix := strings.TrimPrefix(bf.Remote, remotePath)

		b.AddFileToCollection(bf.Local, remotePath+"/", remoteSuffix, bf.Size,
			fileExists(filepath.Join(bf.Local, remoteSuffix)))

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

// BackupTree generates a tree of files that have been backed up via Backup
// Plans.
//
// Requires iRODS environment details and a map of remote collections to their
// corresponding local transformer.
//
// For local path discovery, also takes treedbs of the local system (as given to
// the server subcommand).
//
// The tree returned contains the reverse-engineered local paths of the backup
// up filed.
//
// Each file node contains the size and a boolean that indicates whether or not
// the file exists locally.
//
// The data for a directory nodes contain the following information:
//
//	Total Size of Backed Up Files
//	Total Count of Backed Up Files
//	Total Size of Archived Files
//	Total Count of Archived Files
//
// A claimed directory, in addition, contains the following data:
//
//	Remove Collection Path
//	Collection Tree
//
// The collection tree is a treedb containing only the files that exist for that
// claimed directory. The data for a file node is a `1` if the file exists
// locally, and no data if it does not.
func BackupTree(env iron.Env, collections map[string]transformer.PathTransformer,
	mountTrees ...string) (tree.Node, error) {
	c, err := iron.New(context.Background(), env, iron.Option{
		ClientName:    "backup-plans",
		HandshakeFunc: oldHandshake(env),
	})
	if err != nil {
		return nil, err
	}

	defer c.Close()

	fileExists, cfn, err := openMounts(mountTrees)
	if err != nil {
		return nil, err
	}

	defer cfn()

	return processCollections(c.API, collections, fileExists)
}

func noMounts(string) bool { return true }

type fileCheck func(string) bool

func openMounts(mountTrees []string) (mc fileCheck, c func(), err error) {
	if len(mountTrees) == 0 {
		return noMounts, func() {}, nil
	}

	mounts, c, err := makeMounts(mountTrees)
	if err != nil {
		c()

		return nil, nil, err
	}

	return mountsFunc(mounts), c, nil
}

func makeMounts(mountTree []string) (map[string]*tree.MemTree, func(), error) {
	mounts := make(map[string]*tree.MemTree)

	closers := make([]func(), 0, len(mountTree))

	c := func() {
		for _, closer := range closers {
			closer()
		}
	}

	for _, tree := range mountTree {
		mt, closer, err := memtree.Open(tree)
		if err != nil {
			return nil, c, err
		}

		closers = append(closers, closer)

		root, mp, err := memtree.GetSingleRoot(mt)
		if err != nil {
			return nil, c, err
		}

		mounts[mp] = root
	}

	return mounts, c, nil
}

var noNode tree.MemTree //nolint:gochecknoglobals

func mountsFunc(mounts map[string]*tree.MemTree) fileCheck { //nolint:gocognit,funlen
	cache := make(map[string]*tree.MemTree)

	return func(p string) bool {
		var err error

		dir := path.Dir(p)

		n, ok := cache[dir]
		if !ok { //nolint:nestif
			for mount, node := range mounts {
				if !strings.HasPrefix(p, mount) {
					continue
				}

				for part := range iiter.PathParts(strings.TrimPrefix(p, mount)) {
					node, err = node.Child(part)
					if err != nil {
						node = &noNode

						break
					}
				}

				cache[dir] = node
				n = node //nolint:staticcheck

				break
			}
		}

		if n == nil {
			return false
		}

		_, err = n.Child(path.Base(p))

		return err == nil
	}
}

func processCollections(a *api.API, collections map[string]transformer.PathTransformer,
	fileExists fileCheck) (tree.Node, error) {
	t := newBackupTree()

	for collection, tx := range collections {
		if err := t.AddCollection(a, collection, tx, fileExists); err != nil {
			return nil, err
		}
	}

	return t, nil
}
