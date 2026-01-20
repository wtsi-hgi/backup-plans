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

package ruletree

import (
	"path/filepath"
	"slices"
	"strings"

	"vimagination.zapto.org/tree"
)

// GlobPath returns a sorted slice of all of the paths in the trees that match
// the supplied glob.
func (r *RootDir) GlobPath(path string) []string {
	res := appendAll(nil, "/", r.globPath(path))

	slices.Sort(res)

	return res
}

func (r *RootDir) globPath(path string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.glob(strings.TrimPrefix(path, "/"))
}

// GlobPaths returns a sorted slice of all of the paths in the trees that match
// any of the supplied globs.
func (r *RootDir) GlobPaths(paths ...string) []string {
	var res []string

	for _, path := range paths {
		res = appendAll(res, "/", r.globPath(path))
	}

	slices.Sort(res)

	return slices.Compact(res)
}

func (t *topLevelDir) glob(match string) []string {
	if match == "" {
		return []string{""}
	}

	pattern, rest := splitMatch(match)

	var res []string

	for dir, child := range t.children {
		if globMatch(pattern, dir) {
			res = appendAll(res, dir, child.glob(rest))
		}
	}

	return res
}

func globMatch(pattern, dir string) bool {
	m, _ := filepath.Match(strings.TrimSuffix(pattern, "/"), strings.TrimSuffix(dir, "/")) //nolint:errcheck

	return m && strings.HasSuffix(pattern, "/") == strings.HasSuffix(dir, "/")
}

func splitMatch(match string) (string, string) {
	pos := strings.IndexByte(match, '/')
	if pos == -1 {
		return match, ""
	}

	return match[:pos+1], match[pos+1:]
}

func appendAll(res []string, dir string, children []string) []string {
	for _, child := range children {
		res = append(res, dir+child)
	}

	return res
}

func (r *ruleOverlay) glob(match string) []string {
	if match == "" {
		return []string{""}
	}

	pattern, rest := splitMatch(match)

	var res []string

	for dir, child := range r.lower.Children() {
		if globMatch(pattern, dir) { //nolint:nestif
			if !strings.HasSuffix(dir, "/") {
				res = append(res, dir)
			} else {
				ro := &ruleOverlay{child.(*tree.MemTree), nil} //nolint:errcheck,forcetypeassert
				res = appendAll(res, dir, ro.glob(rest))
			}
		}
	}

	return res
}
