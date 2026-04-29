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

package ruletree

import (
	"errors"
	"iter"
	"strings"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
)

var emptyWildcard = make(group.StateMachine[int64], 2).GetState(nil) //nolint:gochecknoglobals,mnd

type summariser interface {
	Summary(path string, wildcard group.State[int64]) (*DirSummary, error)
	GetOwner(path string) (uint32, uint32, error)
	IsDirectory(path string) bool
	glob(match string) []string
}

// DirRules is a Directory reference and a map of its rules, keyed by the Match.
type DirRules struct {
	*db.Directory

	Rules map[string]*db.Rule
}

// DirRule is a combined Directory reference and Rule reference.
type DirRule struct {
	*db.Directory
	*db.Rule
}

// GetOwner returns the UID and GID for the directory denoted by the given path.
func (r *RootDir) GetOwner(path string) (uint32, uint32, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.topLevelDir.GetOwner(path)
}

type topLevelDir struct {
	parent   *topLevelDir
	children map[string]summariser
	summary  DirSummary
}

func newTopLevelDir(parent *topLevelDir) *topLevelDir {
	return &topLevelDir{
		parent:   parent,
		children: make(map[string]summariser),
		summary: DirSummary{
			Children:      make(map[string]*DirSummary),
			RuleSummaries: make([]Rule, 0),
		},
	}
}

func (t *topLevelDir) setChild(name string, child summariser) error {
	t.children[name] = child

	return t.Update()
}

func (t *topLevelDir) Update() error {
	clear(t.summary.Children)
	t.summary.RuleSummaries = t.summary.RuleSummaries[:0]

	for name, child := range t.children {
		s, err := child.Summary("", emptyWildcard)
		if err != nil {
			return err
		}

		t.summary.mergeRules(s.RuleSummaries)
		t.summary.Children[name] = &DirSummary{
			RuleSummaries: s.RuleSummaries,
		}
	}

	t.summary.setLastMod()

	if t.parent != nil {
		return t.parent.Update()
	}

	return nil
}

func (t *topLevelDir) Summary(path string, wildcard group.State[int64]) (*DirSummary, error) {
	if path == "" {
		return &t.summary, nil
	}

	child, name, rest, err := t.getChild(path)
	if err != nil {
		return nil, err
	}

	return child.Summary(rest, wildcard.GetStateString(name))
}

func (t *topLevelDir) getChild(path string) (summariser, string, string, error) {
	pos := strings.IndexByte(path, '/')

	child := t.children[path[:pos+1]]
	if child == nil {
		return nil, "", "", ErrNotFound
	}

	return child, path[:pos+1], path[pos+1:], nil
}

func (t *topLevelDir) GetOwner(path string) (uint32, uint32, error) {
	if path == "" {
		return 0, 0, nil
	}

	child, _, rest, err := t.getChild(path)
	if err != nil {
		return 0, 0, err
	}

	return child.GetOwner(rest)
}

func (t *topLevelDir) IsDirectory(path string) bool {
	return isDirectory(path, t.getChild)
}

func isDirectory(path string, getChild func(string) (summariser, string, string, error)) bool {
	cr, _, path, err := getChild(path)

	for err != nil {
		return false
	}

	if path == "" {
		return true
	}

	return cr.IsDirectory(path)
}

func createTopLevelDirs(treeRoot *ruleOverlay, rootPath string, p *topLevelDir) error { //nolint:gocognit
	for part := range pathParts(rootPath[1 : len(rootPath)-1]) {
		np, ok := p.children[part]
		if !ok {
			np = newTopLevelDir(p)

			if err := p.setChild(part, np); err != nil {
				return err
			}
		}

		dir, ok := np.(*topLevelDir)
		if !ok {
			return ErrDeepTree
		}

		p = dir
	}

	name := rootPath[strings.LastIndexByte(rootPath[:len(rootPath)-1], '/')+1:]

	if existing, ok := p.children[name]; ok {
		if _, ok = existing.(*ruleOverlay); !ok {
			return ErrDeepTree
		}
	}

	return p.setChild(name, treeRoot)
}

func pathParts(path string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for {
			pos := strings.IndexByte(path, '/')
			if pos == -1 {
				return
			}

			if !yield(path[:pos+1]) {
				break
			}

			path = path[pos+1:]
		}
	}
}

func parentParts(path string) iter.Seq[string] {
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

var (
	ErrDeepTree = errors.New("tree cannot be child of another tree")
	ErrNotFound = errors.New("path not found")
)
