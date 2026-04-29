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
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

// RootDir represents the root of a collection of tree databases and rules.
type RootDir struct {
	topLevelDir

	mu        sync.RWMutex
	rules     *rules.Database
	wildcards map[string]group.State[int64]
	closers   map[string]func()
	claimed   map[string]*DirSummary
	cached    map[string]*DirSummary

	buildMu sync.Mutex
}

// NewRoot create a new RootDir, initialised with the given rules.
func NewRoot(rules *rules.Database) (*RootDir, error) {
	r := &RootDir{
		rules:   rules,
		closers: make(map[string]func()),
		claimed: make(map[string]*DirSummary),
		topLevelDir: topLevelDir{
			children: make(map[string]summariser),
			summary: DirSummary{
				Children:      make(map[string]*DirSummary),
				RuleSummaries: make([]Rule, 0),
			},
		},
		wildcards: make(map[string]group.State[int64]),
	}

	for dir := range rules.Dirs() {
		r.claimed[dir.Path] = nil
	}

	return r, nil
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

func (r *RootDir) ClaimDirectory(path, claimant string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, _ := r.getSummary(path)

	if err := r.rules.ClaimDirectory(path, claimant); err != nil {
		return err
	}

	if s != nil {
		s.ClaimedBy = r.rules.Claimant(path)
	}

	r.claimed[path] = s

	return nil
}

func (r *RootDir) CanClaim(path, claimant string) bool {
	r.mu.RLock()
	defer r.mu.Unlock()

	uid, gids := users.GetIDs(claimant)

	var owner, group uint32

	if s, ok := r.claimed[path]; ok {
		owner = s.uid
		group = s.gid
	} else if s, ok := r.cached[path]; ok {
		owner = s.uid
		group = s.gid
	} else {
		var err error

		owner, group, err = r.topLevelDir.GetOwner(path)
		if err != nil {
			return false
		}
	}

	return uid == owner || slices.Contains(gids, group)
}

func (r *RootDir) PassDirectory(path, claimant string) error {
	return r.rules.PassDirectory(path, claimant)
}

func (r *RootDir) RevokeDirectory(path, claimant string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.rules.ForfeitDirectory(path); err != nil {
		return err
	}

	delete(r.claimed, path)

	return nil
}

// AddRules adds the given rules and regenerates the tree from the top path.
func (r *RootDir) AddRules(dir string, rules []rules.Rule) error {
	return updateRule(r, dir, rules, addRules)
}

func addRules(directoryRules *rules.Database, dir string, rules []rules.Rule) error {
	return directoryRules.AddRules(dir, rules...)
}

func addRule(directoryRules *rules.Database, dir string, rule rules.Rule) error {
	return directoryRules.AddRules(dir, rule)
}

// GetMountPoint will return the mountpoint for the directory given, it will
// return an empty string if none is found.
func (r *RootDir) GetMountPoint(dir string) string {
	for mp := range r.closers {
		if strings.HasPrefix(dir, mp) {
			return mp
		}
	}

	return ""
}

// AddRule adds the given rule to the given directory and regenerates the rule
// summaries.
func (r *RootDir) AddRule(dir string, rule rules.Rule) error {
	return updateRule(r, dir, rule, addRule)
}

func updateRule[T any](r *RootDir, dir string, rule T,
	updateFn func(*rules.Database, string, T) error) error {
	r.buildMu.Lock()
	defer r.buildMu.Unlock()

	// check path exists before starting

	tx := r.rules.RuleTransaction()

	if err := updateFn(tx, dir, rule); err != nil {
		return err
	}

	if err := r.regenRules(r.GetMountPoint(dir), tx, dir); err != nil {
		return err
	}

	return tx.Commit()
}

// RemoveRule remove the given rule from the given directory and regenerates the
// rule summaries.
func (r *RootDir) RemoveRule(dir string, rule *db.Rule) error {
	return updateRule(r, dir, rule, removeRule)
}

func removeRule(directoryRules *rules.Database, dir string, rule *db.Rule) error {
	return directoryRules.RemoveRules(dir, rule.Match)
}

func (r *RootDir) regenRules(mount string, directoryRules *rules.Database, dirs ...string) error {
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

func (r *RootDir) regenRulesFor(t *topLevelDir, child *ruleOverlay, dirs []string, //nolint:funlen
	directoryRules *rules.Database, mount, name string) error {
	sm, wcs, err := generateStatemachineFor(mount, dirs, directoryRules)
	if err != nil {
		return err
	}

	var (
		rd ruleProcessor
		wg sync.WaitGroup
	)

	wg.Add(1)

	rd.process(child.lower, child.upper, sm.GetStateString(mount), &wg)

	var buf bytes.Buffer

	if err = tree.Serialise(&buf, &rd); err != nil {
		return err
	}

	processed, err := tree.OpenMem(buf.Bytes())
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	child.upper = processed
	r.wildcards[mount] = wcs.GetState(nil)

	if err = t.setChild(name, child); err != nil {
		return err
	}

	r.updateCache(dirs...)

	return nil
}

func (r *RootDir) updateCache(dirs ...string) {
	for claimed := range r.claimed {
		for _, dir := range dirs {
			if strings.HasPrefix(claimed, dir) || strings.HasPrefix(dir, claimed) {
				r.claimed[claimed], _ = r.getSummary(claimed)

				break
			}
		}
	}
}

// Summary returns a Dirsummary for the directory denoted by the given path.
func (r *RootDir) Summary(path string) (*DirSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if s := r.claimed[path]; s != nil {
		return s, nil
	} else if s = r.cached[path]; s != nil {
		return s, nil
	}

	return r.getSummary(path)
}

func (r *RootDir) getSummary(path string) (*DirSummary, error) {
	wcs, ok := r.wildcards[r.GetMountPoint(path)]
	if !ok {
		wcs = emptyWildcard
	}

	s, err := r.topLevelDir.Summary(strings.TrimPrefix(path, "/"), wcs.GetStateString("/"))
	if err != nil {
		return nil, err
	}

	s.ClaimedBy = r.rules.Claimant(path)

	return s, nil
}

// AddTree adds a tree database, specified by the given file path, to the
// RootDir, possibly overriding an existing database if they share the same
// root.
func (r *RootDir) AddTree(file string) (string, error) { //nolint:funlen,unparam
	db, closer, err := openDB(file)
	if err != nil {
		return "", err
	}

	defer func() {
		if err != nil {
			closer()
		}
	}()

	treeRoot, rootPath, err := getRoot(db)
	if err != nil {
		return "", err
	}

	r.buildMu.Lock()
	defer r.buildMu.Unlock()

	r.mu.RLock()
	processed, wcs, err := r.processRules(treeRoot, rootPath)
	r.mu.RUnlock()

	if err != nil {
		return "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err = createTopLevelDirs(processed, rootPath, &r.topLevelDir); err != nil {
		return "", err
	}

	if existing, ok := r.closers[rootPath]; ok {
		existing()
	}

	r.closers[rootPath] = closer
	r.wildcards[rootPath] = wcs.GetState(nil)

	r.updateCache(rootPath)

	return rootPath, nil
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

func (r *RootDir) processRules(treeRoot *tree.MemTree, rootPath string) (*ruleOverlay,
	group.StateMachine[int64], error) {
	sm, wcs, err := generateStatemachineFor(rootPath, nil, r.rules)
	if err != nil {
		return nil, nil, err
	}

	var (
		rd ruleProcessor
		wg sync.WaitGroup
	)

	wg.Add(1)

	rd.process(treeRoot, &emptyNode, sm.GetStateString(rootPath), &wg)

	var buf bytes.Buffer

	if err = tree.Serialise(&buf, &rd); err != nil {
		return nil, nil, err
	}

	processed, err := tree.OpenMem(buf.Bytes())
	if err != nil {
		return nil, nil, err
	}

	return &ruleOverlay{lower: treeRoot, upper: processed}, wcs, nil
}

// CacheSummaries grabs the summaries for the given paths and caches them. Cache
// will be updated when changes occur.
//
// Replaces previously cached paths.
func (r *RootDir) CacheSummaries(paths ...string) {
	s := make(map[string]*DirSummary, len(paths))

	r.mu.RLock()

	for _, path := range paths {
		s[path], _ = r.getSummary(path)
	}

	r.mu.RUnlock()

	r.mu.Lock()
	r.cached = s
	r.mu.Unlock()
}

var (
	ErrInvalidDatabase = errors.New("tree database should have a single root child")
	ErrInvalidRoot     = errors.New("invalid root child")
)
