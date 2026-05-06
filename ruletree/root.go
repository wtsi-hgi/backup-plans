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
	"iter"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/memtree"
	"github.com/wtsi-hgi/backup-plans/rules"
	"github.com/wtsi-hgi/backup-plans/users"
	"github.com/wtsi-hgi/wrstat-ui/summary/group"
	"vimagination.zapto.org/tree"
)

// RootDir represents the root of a collection of tree databases and rules.
type RootDir struct {
	topLevelDir

	rules *rules.Database

	mu        sync.RWMutex
	wildcards map[string]group.State[int64]
	closers   map[string]func()
	claimed   map[string]*DirSummary
	cached    map[string]*DirSummary
}

// NewRoot create a new RootDir, initialised with the given rules.
func NewRoot(rules *rules.Database) *RootDir {
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
	// TODO: Seperate rules by collection and non collection and call updateRule for the two different types here?
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

// RemoveRule removes the given rule from the given directory and regenerates the
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

	s, err := r.topLevelDir.Summary(strings.TrimPrefix(path, "/"), wcs.GetStateString("/"))
	if err != nil {
		return nil, err
	}

	s.ClaimedBy = r.rules.Claimant(path)

	for child := range s.Children {
		s.Children[child].ClaimedBy = r.rules.Claimant(path + child)
	}

	return s, nil
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

var (
	ErrInvalidDatabase = errors.New("tree database should have a single root child")
	ErrInvalidRoot     = errors.New("invalid root child")
)

func (r *RootDir) GetCollections() map[int64]*rules.ColRules {
	return r.rules.GetCollections()
}

func (r *RootDir) CreateCollection(c db.Collection) error {
	return updateCollection(r, c, createCollection)
}

func createCollection(directoryRules *rules.Database, c db.Collection) error {
	return directoryRules.CreateCollection(c.Name, c.Description)
}

func (r *RootDir) UpdateCollection(id int64, name, description string) error {
	return r.rules.UpdateCollection(id, name, description)
}

func (r *RootDir) DeleteCollection(id int64) error {
	return r.rules.DeleteCollection(id)
}

func (r *RootDir) CreateCollectionRules(cID int64, rules []*db.CollectionRule) error {
	return updateCollectionRules(r, cID, rules, createCollectionRule)
}

func createCollectionRule(directoryRules *rules.Database, cID int64, rules ...*db.CollectionRule) error {
	return directoryRules.CreateCollectionRule(cID, rules...)
}

// // AddRules adds the given rules and regenerates the tree from the top path.
// func (r *RootDir) AddRules(dir string, rules []rules.Rule) error {
// 	// TODO: Seperate rules by collection and non collection and call updateRule for the two different types here?
// 	return updateRule(r, dir, rules, addRules)
// }

// func addRules(directoryRules *rules.Database, dir string, rules []rules.Rule) error {
// 	return directoryRules.AddRules(dir, rules...)
// }

func updateCollection[T any](r *RootDir, collection T,
	updateFn func(*rules.Database, T) error) error {
	tx := r.rules.RuleTransaction()
	defer tx.Rollback() //nolint:errcheck

	if err := updateFn(tx, collection); err != nil {
		return err
	}

	// TODO: regen rules for only affected mountpoints
	// get a list of all dirs with collection applied
	// get set of mountpoints
	// regenRules for each
	// Could then try to make a regenRulesForMountpoint func that does this in one go so faster if possible

	// if err := r.regenRules(r.GetMountPoint(dir), tx, dir); err != nil {
	// 	return err
	// }

	return tx.Commit()
}

func updateCollectionRules[T any](
	r *RootDir,
	cID int64,
	rules []T,
	updateFn func(*rules.Database, int64, ...T) error,
) error {
	tx := r.rules.RuleTransaction()
	defer tx.Rollback() //nolint:errcheck

	if err := updateFn(tx, cID, rules...); err != nil {
		return err
	}

	// TODO: regen rules for only affected mountpoints
	// get a list of all dirs with collection applied
	// get set of mountpoints
	// regenRules for each
	// Could then try to make a regenRulesForMountpoint func that does this in one go so faster if possible

	// if err := r.regenRules(r.GetMountPoint(dir), tx, dir); err != nil {
	// 	return err
	// }

	return tx.Commit()
}
