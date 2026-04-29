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

package rules

import (
	"errors"
	"iter"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/wtsi-hgi/backup-plans/db"
)

var (
	ErrOrphanedRule        = errors.New("rule found without directory")
	ErrDirectoryNotClaimed = errors.New("directory not claimed")
	ErrNoRule              = errors.New("no matching rule")
	ErrRuleExists          = errors.New("rule already exists for that match string")
	ErrDuplicateRule       = errors.New("cannot add same match twice")
	ErrDirectoryClaimed    = errors.New("directory already claimed")
)

// dirRule is a combined Directory reference and Rule reference.
type dirRule struct {
	*db.Directory
	*db.Rule
}

// dirRules is a Directory reference and a map of its rules, keyed by the Match.
type dirRules struct {
	*db.Directory

	Rules map[string]*db.Rule
}

type Database struct {
	rulesDB *db.DB

	inTx bool
	tx   *sync.Mutex

	mu             *sync.RWMutex
	directoryRules map[string]*dirRules
	dirs           map[uint64]*db.Directory
	rules          map[uint64]*db.Rule
	delayAdd       []*db.Rule
	delayRemove    []*db.Rule
}

func New(rdb *db.DB) (*Database, error) {
	db := &Database{
		rulesDB:        rdb,
		mu:             new(sync.RWMutex),
		tx:             new(sync.Mutex),
		directoryRules: make(map[string]*dirRules),
		dirs:           make(map[uint64]*db.Directory),
		rules:          make(map[uint64]*db.Rule),
	}

	if err := db.loadRules(); err != nil {
		return nil, err
	}

	return db, nil
}

func (d *Database) loadRules() error {
	dirs := make(map[int64]*dirRules)

	if err := d.rulesDB.ReadDirectories().ForEach(func(dir *db.Directory) error {
		dr := &dirRules{
			Directory: dir,
			Rules:     make(map[string]*db.Rule),
		}
		d.directoryRules[dir.Path] = dr
		dirs[dir.ID()] = dr
		d.dirs[uint64(dir.ID())] = dir //nolint:gosec

		return nil
	}); err != nil {
		return err
	}

	return d.rulesDB.ReadRules().ForEach(func(r *db.Rule) error {
		dir, ok := dirs[r.DirID()]
		if !ok {
			return ErrOrphanedRule
		}

		d.rules[uint64(r.ID())] = r //nolint:gosec
		dir.Rules[r.Match] = r

		return nil
	})
}

const (
	defaultFrequency = 7
	month            = time.Hour * 24 * 30
	twoyears         = time.Hour * 24 * 365 * 2
)

func (d *Database) ClaimDirectory(path, claimant string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.directoryRules[path]; ok {
		return ErrDirectoryClaimed
	}

	rd := time.Now().Add(twoyears)

	directory := &db.Directory{
		Path:       path,
		ClaimedBy:  claimant,
		Frequency:  defaultFrequency,
		ReviewDate: rd.Unix(),
		RemoveDate: rd.Add(month).Unix(),
	}

	if err := d.rulesDB.CreateDirectory(directory); err != nil {
		return err
	}

	d.directoryRules[path] = &dirRules{
		Directory: directory,
		Rules:     make(map[string]*db.Rule),
	}
	d.dirs[uint64(directory.ID())] = directory //nolint:gosec

	return nil
}

func (d *Database) PassDirectory(path, claimant string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	directory := d.directoryRules[path]
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	directory.ClaimedBy = claimant

	return d.rulesDB.UpdateDirectory(directory.Directory)
}

func (d *Database) ForfeitDirectory(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	directory := d.directoryRules[path]
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	rules := slices.Collect(maps.Values(directory.Rules))

	if err := d.rulesDB.RemoveRules(rules...); err != nil {
		return err
	}

	for _, rule := range rules {
		delete(d.rules, uint64(rule.ID()))
	}

	if err := d.rulesDB.RemoveDirectory(directory.Directory); err != nil {
		return err
	}

	delete(d.dirs, uint64(directory.ID()))
	delete(d.directoryRules, path)

	return nil
}

func (d *Database) Claimant(path string) string {
	dir, ok := d.directoryRules[path]
	if !ok {
		return ""
	}

	return dir.ClaimedBy
}

type BackupType = db.BackupType

type Rule struct {
	ID          int64
	DirectoryID int64
	BackupType  BackupType
	Metadata    string
	Match       string
	Override    bool
}

func (d *Database) AddRules(path string, rules ...Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	directory := d.directoryRules[path]
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	dbRules := make([]*db.Rule, len(rules))
	adding := make(map[string]struct{})

	for n, rule := range rules {
		if _, ok := directory.Rules[rule.Match]; ok {
			return ErrRuleExists
		} else if _, ok = adding[rule.Match]; ok {
			return ErrDuplicateRule
		}

		adding[rule.Match] = struct{}{}
		dbRules[n] = &db.Rule{
			Match:      rule.Match,
			Metadata:   rule.Metadata,
			BackupType: rule.BackupType,
			Override:   rule.Override,
		}
	}

	if err := d.rulesDB.CreateDirectoryRule(directory.Directory, dbRules...); err != nil {
		return err
	}

	if d.inTx {
		d.delayAdd = append(d.delayAdd, dbRules...)
	} else {
		for _, rule := range dbRules {
			directory.Rules[rule.Match] = rule
			d.rules[uint64(rule.ID())] = rule
		}
	}

	return nil
}

func (d *Database) UpdateRule(path string, rule Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	directory := d.directoryRules[path]
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	dbRule, ok := directory.Rules[rule.Match]
	if !ok {
		return ErrNoRule
	}

	oldMetadata := dbRule.Metadata
	oldBackupType := dbRule.BackupType
	oldOverride := dbRule.Override
	dbRule.Metadata = rule.Metadata
	dbRule.BackupType = rule.BackupType
	dbRule.Override = rule.Override

	if err := d.rulesDB.UpdateRule(dbRule); err != nil {
		dbRule.Metadata = oldMetadata
		dbRule.BackupType = oldBackupType
		dbRule.Override = oldOverride

		return err
	}

	return nil
}

func (d *Database) RemoveRules(path string, matches ...string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	directory := d.directoryRules[path]
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	rules := make([]*db.Rule, len(matches))

	for n, match := range matches {
		rule, ok := directory.Rules[match]
		if !ok {
			return ErrNoRule
		}

		rules[n] = rule
	}

	if d.inTx {
		d.delayRemove = append(d.delayRemove, rules...)
	} else {
		if err := d.rulesDB.RemoveRules(rules...); err != nil {
			return err
		}

		for _, match := range matches {
			delete(directory.Rules, match)
		}
	}

	return nil
}

type Directory struct {
	Path       string
	ClaimedBy  string
	ReviewDate int64
	RemoveDate int64
	Frequency  uint
	Frozen     bool
	Melt       int64
}

func (d *Database) Dirs() iter.Seq[Directory] {
	return func(yield func(Directory) bool) {
		d.mu.RLock()
		defer d.mu.RUnlock()

		for _, dir := range d.dirs {
			if !yield(Directory{
				Path:       dir.Path,
				ClaimedBy:  dir.ClaimedBy,
				ReviewDate: dir.ReviewDate,
				RemoveDate: dir.RemoveDate,
				Frequency:  dir.Frequency,
				Frozen:     dir.Frozen,
				Melt:       dir.Melt,
			}) {
				return
			}
		}
	}
}

func (d *Database) DirRules(path string) iter.Seq[Rule] {
	return func(yield func(Rule) bool) {
		d.mu.RLock()
		defer d.mu.RUnlock()

		directory := d.directoryRules[path]
		if directory == nil {
			return
		}

		for _, rule := range directory.Rules {
			if slices.Contains(d.delayRemove, rule) {
				continue
			}

			if !yield(ToRule(rule)) {
				return
			}
		}

		for _, rule := range d.delayAdd {
			if rule.DirID() != directory.ID() {
				continue
			}

			if !yield(ToRule(rule)) {
				return
			}
		}
	}
}

func (d *Database) Rules() iter.Seq[Rule] {
	return func(yield func(Rule) bool) {
		d.mu.RLock()
		defer d.mu.RUnlock()

		for _, rule := range d.rules {
			if slices.Contains(d.delayRemove, rule) {
				continue
			}

			if !yield(ToRule(rule)) {
				return
			}
		}

		for _, rule := range d.delayAdd {
			if !yield(ToRule(rule)) {
				return
			}
		}
	}
}

func (d *Database) RuleTransaction() *Database {
	if d.inTx {
		return nil
	}

	d.tx.Lock()

	return &Database{
		rulesDB:        d.rulesDB,
		inTx:           true,
		tx:             d.tx,
		mu:             d.mu,
		directoryRules: d.directoryRules,
		dirs:           d.dirs,
		rules:          d.rules,
	}
}

func (d *Database) Commit() error {
	if !d.inTx {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.rulesDB.RemoveRules(d.delayRemove...); err != nil {
		return err
	}

	for _, add := range d.delayAdd {
		dr := d.getDirectoryRules(uint64(add.DirID()))
		if dr == nil {
			continue
		}

		dr.Rules[add.Match] = add
	}

	for _, rm := range d.delayRemove {
		dr := d.getDirectoryRules(uint64(rm.DirID()))
		if dr == nil {
			continue
		}

		delete(dr.Rules, rm.Match)
	}

	d.delayAdd = nil
	d.delayRemove = nil

	d.tx.Unlock()

	return nil
}

func (d *Database) Rollback() error {
	if !d.inTx {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.rulesDB.RemoveRules(d.delayAdd...); err != nil {
		return err
	}

	d.delayAdd = nil
	d.delayRemove = nil

	d.tx.Unlock()

	return nil
}

func (d *Database) getDirectoryRules(id uint64) *dirRules {
	directory := d.dirs[id]
	if directory == nil {
		return nil
	}

	return d.directoryRules[directory.Path]
}

func ToRule(r *db.Rule) Rule {
	return Rule{
		ID:          r.ID(),
		DirectoryID: r.DirID(),
		BackupType:  r.BackupType,
		Match:       r.Match,
		Metadata:    r.Metadata,
		Override:    r.Override,
	}
}
