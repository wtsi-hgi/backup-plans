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
	"fmt"
	"os"

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

// FromTree serialises the given tree to the given path and then calls
// OpenMemTree.
func FromTree(n tree.Node, path string) (*tree.MemTree, func(), error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}

	if err := tree.Serialise(f, n); err != nil {
		f.Close()
		os.Remove(path)

		return nil, nil, err
	}

	if err := f.Close(); err != nil {
		return nil, nil, err
	}

	return Open(path)
}
