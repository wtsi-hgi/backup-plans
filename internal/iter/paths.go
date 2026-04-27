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

package iter

import (
	"iter"
	"strings"
)

// PathParts returns an iterator that returns each directory part including a
// trailing slash.
//
// Path must not contain a starting slash.
func PathParts(path string) iter.Seq[string] {
	return pathParts(path, false)
}

func pathParts(path string, includeFile bool) iter.Seq[string] { //nolint:gocognit
	return func(yield func(string) bool) {
		for path != "" {
			pos := strings.IndexByte(path, '/')
			if pos == -1 {
				if includeFile {
					yield(path)
				}

				return
			}

			if !yield(path[:pos+1]) {
				break
			}

			path = path[pos+1:]
		}
	}
}

// FilePathParts acts like PathParts, but will conclude with a trailing
// filename, if it exists.
func FilePathParts(path string) iter.Seq[string] {
	return pathParts(path, true)
}

// ParentParts acts like PathParts, but reverses the yielded parts.
func ParentParts(path string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for len(path) > 0 {
			pos := strings.LastIndexByte(path[:len(path)-1], '/')
			if pos == -1 {
				return
			}

			if !yield(path[:pos+1]) {
				break
			}

			path = path[:pos]
		}
	}
}
