/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
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

package memtree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

// Open opens the given path as a mmaped tree.
//
// The returned function should be called when the tree is no longer required.
func Open(path string) (*tree.MemTree, func(), error) {
	f, size, err := openAndSize(path)
	if err != nil {
		return nil, nil, err
	}

	data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		f.Close()

		return nil, nil, err
	}

	fn := func() {
		unix.Munmap(data) //nolint:errcheck
		f.Close()
	}

	db, err := tree.OpenMem(data)
	if err != nil {
		fn()

		return nil, nil, fmt.Errorf("error opening tree: %w", err)
	}

	return db, fn, nil
}

func openAndSize(path string) (*os.File, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()

		return nil, 0, err
	}

	return f, int(stat.Size()), nil
}

// InMemory writes the Node bytes into memory and opens it as a MemTree.
func InMemory(n tree.Node) (*tree.MemTree, error) {
	var buf bytes.Buffer

	if err := tree.Serialise(&buf, n); err != nil {
		return nil, err
	}

	return tree.OpenMem(buf.Bytes())
}

// FromTree serialises the given tree to the given path and then calls
// OpenMemTree.
func FromTree(n tree.Node, path string) (*tree.MemTree, func(), error) {
	if err := TreeToFile(n, path); err != nil {
		return nil, nil, err
	}

	return Open(path)
}

// TreeToFile writes the given tree to the stated file path.
func TreeToFile(n tree.Node, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	if err := tree.Serialise(f, n); err != nil {
		f.Close()
		os.Remove(path)

		return err
	}

	return f.Close()
}

// GetSingle root is intended to get the mountpoint for a tree database which is
// stored as the first and only child of the root node.
//
// Returns the mountpoint node and the mountpoint name.
func GetSingleRoot(node *tree.MemTree) (*tree.MemTree, string, error) {
	if node.NumChildren() != 1 {
		return nil, "", ErrInvalidDatabase
	}

	var (
		rootPath string
		treeRoot *tree.MemTree
	)

	node.Children()(func(path string, node tree.Node) bool {
		rootPath = strings.Clone(path)
		treeRoot = node.(*tree.MemTree) //nolint:errcheck,forcetypeassert

		return false
	})

	if !strings.HasPrefix(rootPath, "/") || !strings.HasSuffix(rootPath, "/") {
		return nil, "", ErrInvalidRoot
	}

	return treeRoot, rootPath, nil
}

var (
	ErrInvalidDatabase = errors.New("tree database should have a single root child")
	ErrInvalidRoot     = errors.New("invalid root child")
)
