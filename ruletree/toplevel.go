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
	"os"
	"path"
	"strings"
	"sync"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

const nameBufLen = 4096

type summariser interface {
	Summary(path string) (*DirSummary, error)
	GetOwner(path string) (uint32, uint32, error)
}

// DirRules is a Directory reference and a map of its rules, keyed by the Match.
type DirRules struct {
	*db.Directory

	Rules map[string]*db.Rule
}

// RootDir represents the root of a collection of tree databases and rules.
type RootDir struct {
	topLevelDir

	directoryRules map[string]*DirRules
	stateMachine   group.StateMachine[db.Rule]

	mu      sync.RWMutex
	closers map[string]func()
}

// DirRule is a combined Directory reference and Rule reference.
type DirRule struct {
	*db.Directory
	*db.Rule
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
		if err := r.addRule(dr.Directory, dr.Rule); err != nil {
			return nil, err
		}
	}

	err := r.rebuildStateMachine()

	return r, err
}

// AddRule adds the given rule to the given directory and regenerates the rule
// summaries.
func (r *RootDir) AddRule(dir *db.Directory, rule *db.Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.addRule(dir, rule); err != nil {
		return err
	}

	return r.regenRules(dir.Path)
}

func (r *RootDir) addRule(dir *db.Directory, rule *db.Rule) error {
	existingDir, ok := r.directoryRules[dir.Path]
	if !ok {
		existingDir = &DirRules{
			Directory: dir,
			Rules:     make(map[string]*db.Rule),
		}

		r.directoryRules[dir.Path] = existingDir
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
	r.mu.Lock()
	defer r.mu.Unlock()

	existingDir, ok := r.directoryRules[dir.Path]
	if !ok {
		return ErrNotFound
	}

	if _, ruleExists := existingDir.Rules[rule.Match]; !ruleExists {
		return ErrRuleNotFound
	}

	delete(existingDir.Rules, rule.Match)

	return r.regenRules(dir.Path)
}

func (r *RootDir) regenRules(dir string) error {
	t := &r.topLevelDir
	pos := 1

	for part := range pathParts(dir[1:]) {
		child := t.children[part]
		if child == nil {
			return ErrNotFound
		}

		pos += len(part)

		switch child := child.(type) {
		case *topLevelDir:
			t = child
		case *ruleOverlay:
			return r.regenRulesFor(t, child, dir, dir[:pos], part)
		default:
			return ErrNotFound
		}
	}

	return ErrNotFound
}

func (r *RootDir) regenRulesFor(t *topLevelDir, child *ruleOverlay, dir, curr, name string) error {
	if err := r.rebuildStateMachine(); err != nil {
		return err
	}

	rd := ruleLessDirPatch{
		rulesDir: rulesDir{
			node: child.lower,
			sm:   r.stateMachine.GetStateString(curr),
		},
		ruleDirPrefixes: r.createRulePatchMap(dir),
		previousRules:   child.upper,
		nameBuf:         append(make([]byte, 0, nameBufLen), curr...),
	}

	var buf bytes.Buffer

	if err := tree.Serialise(&buf, &rd); err != nil {
		return err
	}

	processed, err := tree.OpenMem(buf.Bytes())
	if err != nil {
		return err
	}

	child.upper = processed

	return t.setChild(name, child)
}

func (r *RootDir) rebuildStateMachine() error {
	var ruleList []group.PathGroup[db.Rule]

	for dir, rules := range r.directoryRules {
		for _, rule := range rules.Rules {
			ruleList = append(ruleList, group.PathGroup[db.Rule]{
				Path:  []byte(path.Join(dir, rule.Match)),
				Group: rule,
			})
		}
	}

	sm, err := group.NewStatemachine(ruleList)
	if err != nil {
		return err
	}

	r.stateMachine = sm

	return nil
}

func (r *RootDir) createRulePatchMap(dir string) map[string]bool {
	rulePrefixes := make(map[string]bool)
	rulePrefixes[dir] = true

	for dir != "/" {
		pos := strings.LastIndexByte(dir[:len(dir)-1], '/')
		dir = dir[:pos+1]
		rulePrefixes[dir] = false
	}

	return rulePrefixes
}

// Summary returns a Dirsummary for the directory denoted by the given path.
func (r *RootDir) Summary(path string) (*DirSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.topLevelDir.Summary(path)
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
		s, err := child.Summary("")
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

func (t *topLevelDir) Summary(path string) (*DirSummary, error) {
	if path == "" {
		return &t.summary, nil
	}

	child, rest, err := t.getChild(path)
	if err != nil {
		return nil, err
	}

	return child.Summary(rest)
}

func (t *topLevelDir) getChild(path string) (summariser, string, error) { //nolint:ireturn
	pos := strings.IndexByte(path, '/')

	child := t.children[path[:pos+1]]
	if child == nil {
		return nil, "", ErrNotFound
	}

	return child, path[pos+1:], nil
}

func (t *topLevelDir) GetOwner(path string) (uint32, uint32, error) {
	if path == "" {
		return 0, 0, nil
	}

	child, rest, err := t.getChild(path)
	if err != nil {
		return 0, 0, err
	}

	return child.GetOwner(rest)
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

	r.mu.Lock()
	defer r.mu.Unlock()

	processed, err := r.processRules(treeRoot, rootPath)
	if err != nil {
		return err
	}

	if err = createTopLevelDirs(processed, rootPath, &r.topLevelDir); err != nil {
		return err
	}

	if existing, ok := r.closers[rootPath]; ok {
		existing()
	}

	r.closers[rootPath] = closer

	return nil
}

func openDB(file string) (*tree.MemTree, func(), error) { //nolint:funlen
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close() // nolint:errcheck

		return nil, nil, err
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(stat.Size()), unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		f.Close() // nolint:errcheck

		return nil, nil, err
	}

	fn := func() {
<<<<<<< HEAD
		unix.Munmap(data) //nolint:errcheck
		f.Close()
=======
		unix.Munmap(data) // nolint:errcheck
		f.Close()         // nolint:errcheck
>>>>>>> 0544c03 (Add Makefile; delint)
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
		rootPath = path
		treeRoot = node.(*tree.MemTree) //nolint:errcheck,forcetypeassert

		return false
	})

	if !strings.HasPrefix(rootPath, "/") || !strings.HasSuffix(rootPath, "/") {
		return nil, "", ErrInvalidRoot
	}

	return treeRoot, rootPath, nil
}

func (r *RootDir) processRules(treeRoot *tree.MemTree, rootPath string) (*ruleOverlay, error) {
	rd := ruleLessDir{
		rulesDir: rulesDir{
			node: treeRoot,
			sm:   r.stateMachine.GetStateString(rootPath),
		},
		ruleDirPrefixes: r.createRulePrefixMap(rootPath),
		rules:           new(dirWithRule),
		nameBuf:         append(make([]byte, 0, nameBufLen), rootPath...),
	}

	var buf bytes.Buffer

	if err := tree.Serialise(&buf, &rd); err != nil {
		return nil, err
	}

	processed, err := tree.OpenMem(buf.Bytes())
	if err != nil {
		return nil, err
	}

	return &ruleOverlay{lower: treeRoot, upper: processed}, nil
}

func (r *RootDir) createRulePrefixMap(rootPath string) map[string]bool {
	rulePrefixes := make(map[string]bool)

	for ruleDir := range r.directoryRules {
		if !strings.HasPrefix(ruleDir, rootPath) {
			continue
		}

		rulePrefixes[ruleDir] = true

		for ruleDir != "/" {
			ruleDir = ruleDir[:strings.LastIndexByte(ruleDir[:len(ruleDir)-1], '/')+1]
			rulePrefixes[ruleDir] = rulePrefixes[ruleDir] || false
		}
	}

	return rulePrefixes
}

func createTopLevelDirs(treeRoot *ruleOverlay, rootPath string, p *topLevelDir) error { //nolint:gocognit
	for part := range pathParts(rootPath[1 : len(rootPath)-1]) {
		np, ok := p.children[part]
		if !ok {
			np = newTopLevelDir(p)

<<<<<<< HEAD
			if err := p.setChild(part, np); err != nil {
=======
			err := p.setChild(part, np)
			if err != nil {
>>>>>>> 0544c03 (Add Makefile; delint)
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
