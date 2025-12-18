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
	"bytes"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"strings"
	"sync"

	"github.com/wtsi-hgi/backup-plans/db"
	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

type summariser interface {
	Summary(path string, wildcard *wildcards) (*DirSummary, error)
	GetOwner(path string) (uint32, uint32, error)
	IsDirectory(path string) bool
}

// DirRules is a Directory reference and a map of its rules, keyed by the Match.
type DirRules struct {
	*db.Directory

	Rules map[string]*db.Rule
}

// RootDir represents the root of a collection of tree databases and rules.
type RootDir struct {
	topLevelDir

	mu             sync.RWMutex
	directoryRules map[string]*DirRules
	wildcards      wildcards
	closers        map[string]func()

	buildMu sync.Mutex
}

// DirRule is a combined Directory reference and Rule reference.
type DirRule struct {
	*db.Directory
	*db.Rule
}

// IsDirectory returns whether a given path is a path to a directory.
func (r *RootDir) IsDirectory(path string) bool {
	if strings.HasSuffix(path, "/") {
		return true
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.topLevelDir.IsDirectory(strings.TrimPrefix(path, "/") + "/")
}

// NewRoot create a new RootDir, initialised with the given rules.
func NewRoot(rules []DirRule) (*RootDir, error) {
	r := &RootDir{
		directoryRules: make(map[string]*DirRules),
		closers:        make(map[string]func()),
		topLevelDir: topLevelDir{
			children: make(map[string]summariser),
			summary: DirSummary{
				Children:      make(map[string]*DirSummary),
				RuleSummaries: make([]Rule, 0),
			},
		},
	}

	for _, dr := range rules {
		if err := addRule(r.directoryRules, dr.Directory, dr.Rule); err != nil {
			return nil, err
		}
	}

	r.buildWildcards()

	return r, nil
}

// AddRules adds the given rules and regenerates the tree from the top path.
func (r *RootDir) AddRules(topPath string, dirRules []DirRule) error {
	r.buildMu.Lock()
	defer r.buildMu.Unlock()

	r.mu.RLock()
	directoryRules := maps.Clone(r.directoryRules)
	r.mu.RUnlock()

	dirs, err := addRules(directoryRules, dirRules)
	if err != nil {
		return err
	}

	if err := r.regenRules(r.getMountPoint(topPath), directoryRules, dirs...); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.directoryRules = directoryRules

	return nil
}
func addRules(directoryRules map[string]*DirRules, dirRules []DirRule) ([]string, error) {
	dirs := make([]string, 0, len(dirRules))

	for _, rule := range dirRules {
		if err := addRule(directoryRules, rule.Directory, rule.Rule); err != nil {
			return nil, err
		}

		dirs = append(dirs, rule.Path)
	}

	return dirs, nil
}

func (r *RootDir) getMountPoint(dir string) string {
	for mp := range r.closers {
		if strings.HasPrefix(dir, mp) {
			return mp
		}
	}

	return ""
}

// AddRule adds the given rule to the given directory and regenerates the rule
// summaries.
func (r *RootDir) AddRule(dir *db.Directory, rule *db.Rule) error {
	return r.updateRule(dir, rule, addRule)
}

func (r *RootDir) updateRule(dir *db.Directory, rule *db.Rule,
	updateFn func(map[string]*DirRules, *db.Directory, *db.Rule) error) error {
	r.buildMu.Lock()
	defer r.buildMu.Unlock()

	r.mu.RLock()
	directoryRules := make(map[string]*DirRules, len(r.directoryRules))

	for path, dr := range r.directoryRules {
		if len(dr.Rules) == 0 {
			continue
		}

		directoryRules[path] = &DirRules{
			Directory: dr.Directory,
			Rules:     maps.Clone(dr.Rules),
		}
	}

	r.mu.RUnlock()

	if err := updateFn(directoryRules, dir, rule); err != nil {
		return err
	}

	if err := r.regenRules(r.getMountPoint(dir.Path), directoryRules, dir.Path); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.directoryRules = directoryRules

	return nil
}

func addRule(directoryRules map[string]*DirRules, dir *db.Directory, rule *db.Rule) error {
	existingDir, ok := directoryRules[dir.Path]
	if !ok {
		existingDir = &DirRules{
			Directory: dir,
			Rules:     make(map[string]*db.Rule),
		}

		directoryRules[dir.Path] = existingDir
	}

	if _, ruleExists := existingDir.Rules[rule.Match]; ruleExists {
		return ErrRuleExists
	}

	existingDir.Rules[rule.Match] = rule

	return nil
}

// RemoveRule remove the given rule from the given directory and regenerates the
// rule summaries.
func (r *RootDir) RemoveRule(dir *db.Directory, rule *db.Rule) error {
	return r.updateRule(dir, rule, removeRule)
}

func removeRule(directoryRules map[string]*DirRules, dir *db.Directory, rule *db.Rule) error {
	existingDir, ok := directoryRules[dir.Path]
	if !ok {
		return ErrNotFound
	}

	if _, ruleExists := existingDir.Rules[rule.Match]; !ruleExists {
		return ErrRuleNotFound
	}

	delete(existingDir.Rules, rule.Match)

	return nil
}

func (r *RootDir) regenRules(mount string, directoryRules map[string]*DirRules, dirs ...string) error {
	t := &r.topLevelDir
	pos := 1

	for part := range pathParts(mount[1:]) {
		child := t.children[part]
		if child == nil {
			return ErrNotFound
		}

		pos += len(part)

		switch child := child.(type) {
		case *topLevelDir:
			t = child
		case *ruleOverlay:
			return r.regenRulesFor(t, child, dirs, directoryRules, mount, part)
		default:
			return ErrNotFound
		}
	}

	return ErrNotFound
}

func (r *RootDir) regenRulesFor(t *topLevelDir, child *ruleOverlay, dirs []string,
	directoryRules map[string]*DirRules, mount, name string) error {
	sm, err := generateStatemachineFor(mount, dirs, directoryRules)
	if err != nil {
		return err
	}

	rd := ruleProcessor{lowerNode: child.lower, upperNode: child.upper, sm: sm.GetStateString(mount)}

	var buf bytes.Buffer

	if err = tree.Serialise(&buf, &rd); err != nil {
		return err
	}

	processed, err := tree.OpenMem(buf.Bytes())
	if err != nil {
		return err
	}

	child.upper = processed

	r.mu.Lock()
	defer r.mu.Unlock()

	r.directoryRules = directoryRules

	defer r.buildWildcards()

	return t.setChild(name, child)
}

// Summary returns a Dirsummary for the directory denoted by the given path.
func (r *RootDir) Summary(path string) (*DirSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.topLevelDir.Summary(strings.TrimPrefix(path, "/"), &r.wildcards)
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
		s, err := child.Summary("", nil)
		if err != nil {
			return err
		}

		t.summary.mergeRules(s.RuleSummaries)
		t.summary.Children[name] = &DirSummary{
			RuleSummaries: s.RuleSummaries,
		}
	}

	if t.parent != nil {
		return t.parent.Update()
	}

	return nil
}

func (t *topLevelDir) Summary(path string, wildcard *wildcards) (*DirSummary, error) {
	if path == "" {
		return &t.summary, nil
	}

	child, name, rest, err := t.getChild(path)
	if err != nil {
		return nil, err
	}

	return child.Summary(rest, wildcard.Child(name))
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

// AddTree adds a tree database, specified by the given file path, to the
// RootDir, possibly overriding an existing database if they share the same
// root.
func (r *RootDir) AddTree(file string) (err error) { //nolint:funlen
	db, closer, err := openDB(file)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			closer()
		}
	}()

	treeRoot, rootPath, err := getRoot(db)
	if err != nil {
		return err
	}

	r.buildMu.Lock()
	defer r.buildMu.Unlock()

	r.mu.RLock()
	processed, err := r.processRules(treeRoot, rootPath)
	r.mu.RUnlock()

	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err = createTopLevelDirs(processed, rootPath, &r.topLevelDir); err != nil {
		return err
	}

	if existing, ok := r.closers[rootPath]; ok {
		existing()
	}

	r.closers[rootPath] = closer

	r.buildWildcards()

	return nil
}

func openDB(file string) (*tree.MemTree, func(), error) { //nolint:funlen
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()

		return nil, nil, err
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(stat.Size()), unix.PROT_READ, unix.MAP_SHARED)
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

func getRoot(db *tree.MemTree) (*tree.MemTree, string, error) {
	if db.NumChildren() != 1 {
		return nil, "", ErrInvalidDatabase
	}

	var (
		rootPath string
		treeRoot *tree.MemTree
	)

	db.Children()(func(path string, node tree.Node) bool {
		rootPath = strings.Clone(path)
		treeRoot = node.(*tree.MemTree) //nolint:errcheck,forcetypeassert

		return false
	})

	if !strings.HasPrefix(rootPath, "/") || !strings.HasSuffix(rootPath, "/") {
		return nil, "", ErrInvalidRoot
	}

	return treeRoot, rootPath, nil
}

func (r *RootDir) processRules(treeRoot *tree.MemTree, rootPath string) (*ruleOverlay, error) {
	sm, err := generateStatemachineFor(rootPath, nil, r.directoryRules)
	if err != nil {
		return nil, err
	}

	rd := ruleProcessor{
		lowerNode: treeRoot,
		upperNode: &emptyNode,
		sm:        sm.GetStateString(rootPath),
	}

	var buf bytes.Buffer

	if err = tree.Serialise(&buf, &rd); err != nil {
		return nil, err
	}

	processed, err := tree.OpenMem(buf.Bytes())
	if err != nil {
		return nil, err
	}

	return &ruleOverlay{lower: treeRoot, upper: processed}, nil
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

var (
	ErrInvalidDatabase = errors.New("tree database should have a single root child")
	ErrInvalidRoot     = errors.New("invalid root child")
	ErrDeepTree        = errors.New("tree cannot be child of another tree")
	ErrNotFound        = errors.New("path not found")
	ErrRuleNotFound    = errors.New("rule not found")
	ErrRuleExists      = errors.New("rule already exists for that match string")
)
