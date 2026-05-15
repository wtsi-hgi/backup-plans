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
	"cmp"
	"errors"
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"

	iiter "github.com/wtsi-hgi/backup-plans/internal/iter"
	"github.com/wtsi-hgi/backup-plans/internal/memtree"
	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/byteio"
	"vimagination.zapto.org/tree"
)

type dbCloser struct {
	db     *tree.MemTree
	closer func()
}

// RootDir represents the root of a collection of tree databases and rules.
type RootDir struct {
	topLevelDir

	rules *rules.Database

	mu        sync.RWMutex
	wildcards map[string]group.State[int64]
	trees     map[string]dbCloser
	claimed   map[string]*DirSummary
	cached    map[string]*DirSummary
	backups   *tree.MemTree
}

// NewRoot create a new RootDir, initialised with the given rules.
func NewRoot(rules *rules.Database) *RootDir {
	r := &RootDir{
		rules:   rules,
		trees:   make(map[string]dbCloser),
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

	return r
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

	s, _ := r.getSummary(path) //nolint:errcheck

	if err := r.rules.ClaimDirectory(path, claimant); err != nil {
		return err
	}

	if s != nil {
		s.ClaimedBy = claimant
	}

	r.claimed[path] = s

	if _, ok := r.cached[path]; ok {
		r.cached[path] = s
	}

	r.setClaimed(path, claimant)

	return nil
}

func (r *RootDir) setClaimed(path, claimant string) { //nolint:gocyclo
	if claimed := r.claimed[path]; claimed != nil {
		claimed.ClaimedBy = claimant
	}

	if cached := r.cached[path]; cached != nil {
		cached.ClaimedBy = claimant
	}

	parentPath, childPath := splitPath(path)
	if parentPath == "" {
		return
	}

	if claimed := r.claimed[parentPath]; claimed != nil {
		if child := claimed.Children[childPath]; child != nil {
			child.ClaimedBy = claimant
		}
	}

	if cached := r.cached[parentPath]; cached != nil {
		if child := cached.Children[childPath]; child != nil {
			child.ClaimedBy = claimant
		}
	}
}

func splitPath(path string) (string, string) {
	parentPathPos := strings.LastIndexByte(path[:len(path)-1], '/')
	if parentPathPos < 0 {
		return "", ""
	}

	return path[:parentPathPos+1], path[parentPathPos+1:]
}

// CanClaim returns true if the given username can claim the given path.
func (r *RootDir) CanClaim(path, claimant string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	uid, gids := users.GetIDs(claimant)

	var owner, group uint32

	if s, ok := r.claimed[path]; ok { //nolint:nestif
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

// ClaimedDirectory retrieves the directory details for the specified directory.
func (r *RootDir) ClaimedDirectory(path string) *rules.Directory {
	return r.rules.DirDetails(path)
}

// RuleDir returns the directory details for the directory the Given Rule ID
// belongs to.
func (r *RootDir) RuleDir(id uint64) *rules.Directory {
	return r.rules.RuleDir(id)
}

// PassDirectory will set the claimant of a currently claimed directory to the
// new username provided.
func (r *RootDir) PassDirectory(path, claimant string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.rules.PassDirectory(path, claimant); err != nil {
		return err
	}

	r.setClaimed(path, claimant)

	return nil
}

// RevokeDirectory revokes a claim on a directory.
func (r *RootDir) RevokeDirectory(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.rules.ForfeitDirectory(path); err != nil {
		return err
	}

	delete(r.claimed, path)

	r.setClaimed(path, "")

	return nil
}

// ClaimedDirectories returns an iterator over all of the claimed directories.
func (r *RootDir) ClaimedDirectories() iter.Seq[rules.Directory] {
	return r.rules.Dirs()
}

// ClaimedSummaries returns an iterator for each claimed directory, returning
// the path and a summary.
func (r *RootDir) ClaimedSummaries() iter.Seq2[string, *DirSummary] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return maps.All(maps.Clone(r.claimed))
}

// SetDirDetails sets the details for a directory.
func (r *RootDir) SetDirDetails(dir rules.Directory) error {
	return r.rules.SetDirDetails(dir)
}

// Refreeze resets the melt attribute on the directory.
func (r *RootDir) Refreeze(path string) error {
	return r.rules.Refreeze(path)
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
	for mp := range r.trees {
		if mp == "" {
			continue
		}

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
	tx := r.rules.RuleTransaction()
	defer tx.Rollback() //nolint:errcheck

	if err := updateFn(tx, dir, rule); err != nil {
		return err
	}

	if err := r.regenRules(r.GetMountPoint(dir), tx, dir); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *RootDir) UpdateRule(path string, rule rules.Rule) error {
	return r.rules.UpdateRule(path, rule)
}

// RemoveRule remove the given rule from the given directory and regenerates the
// rule summaries.
func (r *RootDir) RemoveRule(dir string, rule string) error {
	return updateRule(r, dir, rule, removeRule)
}

func removeRule(directoryRules *rules.Database, dir string, rule string) error {
	return directoryRules.RemoveRules(dir, rule)
}

func (r *RootDir) regenRules(mount string, directoryRules *rules.Database, dirs ...string) error {
	t := &r.topLevelDir
	pos := 1

	for part := range iiter.PathParts(mount[1:]) {
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

	rd.process(treeNode{
		child.lower,
		cmp.Or(child.upper, &emptyNode),
		getBackupDir(r.trees[""].db, mount),
	}, sm.GetStateString(mount), &wg)

	processed, err := memtree.InMemory(&rd)
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

func (r *RootDir) updateCache(dirs ...string) { //nolint:gocognit,gocyclo
	for claimed := range r.claimed {
		for _, dir := range dirs {
			if strings.HasPrefix(claimed, dir) || strings.HasPrefix(dir, claimed) {
				s, _ := r.getSummary(claimed) //nolint:errcheck

				r.claimed[claimed] = s

				break
			}
		}
	}

	for cached := range r.cached {
		for _, dir := range dirs {
			if strings.HasPrefix(cached, dir) || strings.HasPrefix(dir, cached) { //nolint:nestif
				if claimed, ok := r.claimed[cached]; ok {
					r.cached[cached] = claimed
				} else {
					r.cached[cached], _ = r.getSummary(cached) //nolint:errcheck
				}
			}
		}
	}
}

// Rule returns the rule details for the Rule designated by the supplied ID.
func (r *RootDir) Rule(id uint64) *rules.Rule {
	return r.rules.Rule(id)
}

// DirRules returns an iterator over all of the rules for a claimed directory.
func (r *RootDir) DirRules(path string) iter.Seq[rules.Rule] {
	return r.rules.DirRules(path)
}

// HasRules returns true if the specified path is a claimed directory with at
// least one rule.
func (r *RootDir) HasRules(path string) bool {
	return r.rules.HasRules(path)
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

	s, err := r.topLevelDir.Summary(
		strings.TrimPrefix(path, "/"),
		wcs.GetStateString("/"),
		cmp.Or(r.trees[""].db, &emptyNode),
	)
	if err != nil {
		return nil, err
	}

	s.ClaimedBy = r.rules.Claimant(path)

	for child := range s.Children {
		s.Children[child].ClaimedBy = r.rules.Claimant(path + child)
	}

	return s, nil
}

type rulesAndWildcards struct {
	processed *ruleOverlay
	wcs       group.StateMachine[int64]
}

func (r *RootDir) SetBackupTree(file string) error {
	db, closer, err := memtree.Open(file)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			closer()
		}
	}()

	defer r.rules.RuleTransaction().Rollback() //nolint:errcheck

	newRoots := make(map[string]rulesAndWildcards)

	for rootPath, tree := range r.trees {
		r.mu.RLock()
		processed, wcs, err := r.processRules(tree.db, db, rootPath)
		r.mu.RUnlock()

		if err != nil {
			return err
		}

		newRoots[rootPath] = rulesAndWildcards{processed, wcs}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for rootPath, rw := range newRoots {
		if err = createTopLevelDirs(rw.processed, rootPath, &r.topLevelDir); err != nil {
			return err
		}

		r.wildcards[rootPath] = rw.wcs.GetState(nil)
		r.updateCache(rootPath)
	}

	r.setTreeCloser("", db, closer)

	r.backups = db

	return nil
}

func (r *RootDir) setTreeCloser(rootPath string, db *tree.MemTree, closer func()) {
	if existing, ok := r.trees[rootPath]; ok {
		existing.closer()
	}

	r.trees[rootPath] = dbCloser{db, closer}
}

// AddTree adds a tree database, specified by the given file path, to the
// RootDir, possibly overriding an existing database if they share the same
// root.
func (r *RootDir) AddTree(file string) (string, error) { //nolint:funlen,unparam
	db, closer, err := memtree.Open(file)
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

	defer r.rules.RuleTransaction().Rollback() //nolint:errcheck

	r.mu.RLock()
	processed, wcs, err := r.processRules(treeRoot, r.trees[""].db, rootPath)
	r.mu.RUnlock()

	if err != nil {
		return "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err = createTopLevelDirs(processed, rootPath, &r.topLevelDir); err != nil {
		return "", err
	}

	r.wildcards[rootPath] = wcs.GetState(nil)

	r.setTreeCloser(rootPath, db, closer)
	r.updateCache(rootPath)

	return rootPath, nil
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

func getBackupDir(backups *tree.MemTree, dir string) *tree.MemTree {
	if backups == nil {
		return &emptyNode
	}

	for part := range iiter.PathParts(strings.TrimPrefix(dir, "/")) {
		backups, _ = backups.Child(part)

		if backups == nil {
			return &emptyNode
		}
	}

	return backups
}

func (r *RootDir) processRules(treeRoot, backups *tree.MemTree, rootPath string) (*ruleOverlay,
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

	rd.process(treeNode{
		treeRoot,
		&emptyNode,
		getBackupDir(backups, rootPath),
	}, sm.GetStateString(rootPath), &wg)

	processed, err := memtree.InMemory(&rd)
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
		if claimed, ok := r.claimed[path]; ok {
			s[path] = claimed
		} else {
			s[path], _ = r.getSummary(path) //nolint:errcheck
		}
	}

	r.mu.RUnlock()

	r.mu.Lock()
	r.cached = s
	r.mu.Unlock()
}

func (r *RootDir) BackedUpFiles(path string) *iiter.IterErr[string] {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := cmp.Or(r.trees[""].db, &emptyNode)

	for part := range iiter.PathParts(strings.TrimPrefix(path, "/")) {
		m, _ := n.Child(part)

		n = cmp.Or(m, &emptyNode)
	}

	lr := byteio.MemLittleEndian(n.Data())

	lr.ReadUintX()
	lr.ReadUintX()

	if len(lr) == 0 {
		return &iiter.IterErr[string]{Error: ErrNoBackups}
	}

	backups, err := tree.OpenMem(lr)
	if err != nil {
		return &iiter.IterErr[string]{Error: err}
	}

	return &iiter.IterErr[string]{
		Iter: walkBackups(backups),
	}
}

func walkBackups(n *tree.MemTree) iter.Seq[string] {
	return func(yield func(string) bool) {
		walkTree(n, []byte{'/'}, yield)
	}
}

func walkTree(n *tree.MemTree, path []byte, yield func(string) bool) bool {
	for child, n := range n.Children() {
		name := append(path, child...)

		if strings.HasSuffix(child, "/") {
			if !walkTree(n.(*tree.MemTree), name, yield) {
				return false
			}
		} else if !yield(string(name)) {
			return false
		}
	}

	return true
}

var (
	ErrInvalidDatabase = errors.New("tree database should have a single root child")
	ErrInvalidRoot     = errors.New("invalid root child")
	ErrNoBackups       = errors.New("no backups for that path")
)
