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

type dirRules struct {
	*db.Directory

	Rules map[string]*db.Rule
}

// Database contains all of the claimed directories and their rules, adding
// convenient, cached access and control.
type Database struct {
	rulesDB *db.DB

	inTx bool
	tx   *sync.Mutex

	mu             *sync.RWMutex
	directoryRules map[string]*dirRules
	dirs           map[uint64]*dirRules
	rules          map[uint64]*db.Rule
	delayAdd       []*db.Rule
	delayRemove    []*db.Rule
}

// New takes a database connection and caches the information for fast access.
func New(rdb *db.DB) (*Database, error) {
	db := &Database{
		rulesDB:        rdb,
		mu:             new(sync.RWMutex),
		tx:             new(sync.Mutex),
		directoryRules: make(map[string]*dirRules),
		dirs:           make(map[uint64]*dirRules),
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
		d.dirs[uint64(dir.ID())] = dr //nolint:gosec

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

// ClaimDirectory will claim the given directory on behalf of the supplied user.
//
// The Frequency will be set to 7 days, the review date set for 2 years time,
// and the remove data 1 month after that.
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

	dr := &dirRules{
		Directory: directory,
		Rules:     make(map[string]*db.Rule),
	}
	d.directoryRules[path] = dr
	d.dirs[uint64(directory.ID())] = dr //nolint:gosec

	return nil
}

// PassDirectory will set the claimaint of a currently claimed directory to the
// new username provided.
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

// ForfeitDirectory revokes a claim on a directory.
func (d *Database) ForfeitDirectory(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	directory := d.directoryRules[path]
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	for _, rule := range directory.Rules {
		delete(d.rules, uint64(rule.ID())) //nolint:gosec
	}

	if err := d.rulesDB.RemoveDirectory(directory.Directory); err != nil {
		return err
	}

	delete(d.dirs, uint64(directory.ID())) //nolint:gosec
	delete(d.directoryRules, path)

	return nil
}

// DirDetails retrieves the directory details for the specified directory.
func (d *Database) DirDetails(path string) *Directory {
	d.mu.RLock()
	defer d.mu.RUnlock()

	directory := d.directoryRules[path]
	if directory == nil {
		return nil
	}

	dir := toDir(directory.Directory)

	return &dir
}

// RuleDir returns the directory details for the directory the Given Rule ID
// belongs to.
func (d *Database) RuleDir(id uint64) *Directory {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rule := d.rules[id]
	if rule == nil {
		return nil
	}

	directory := d.dirs[uint64(rule.DirID())] //nolint:gosec
	if directory == nil {
		return nil
	}

	dir := toDir(directory.Directory)

	return &dir
}

// SetDirDetails sets the details for a directory.
func (d *Database) SetDirDetails(dDetails Directory) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	directory := d.directoryRules[dDetails.Path]
	if directory == nil {
		return ErrDirectoryNotClaimed
	}

	directory.ReviewDate = dDetails.ReviewDate
	directory.RemoveDate = dDetails.RemoveDate
	directory.Frequency = dDetails.Frequency
	directory.Frozen = dDetails.Frozen
	directory.Melt = dDetails.Melt

	return d.rulesDB.UpdateDirectory(directory.Directory)
}

// Claimant returns the username that owns the given directory.
func (d *Database) Claimant(path string) string {
	dir, ok := d.directoryRules[path]
	if !ok {
		return ""
	}

	return dir.ClaimedBy
}

// Refreeze resets the melt attribute on the directory.
func (d *Database) Refreeze(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dir, ok := d.directoryRules[path]
	if !ok {
		return ErrDirectoryClaimed
	}

	if err := d.rulesDB.Refreeze(dir.Directory); err != nil {
		return err
	}

	dir.Melt = 0

	return nil
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

// AddRules adds the supplied rules to the claimed directory specified.
func (d *Database) AddRules(path string, rules ...Rule) error { //nolint:gocyclo,funlen
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
		dbRules[n] = toDBRule(rule)
	}

	if err := d.rulesDB.CreateDirectoryRule(directory.Directory, dbRules...); err != nil {
		return err
	}

	if d.inTx {
		d.delayAdd = append(d.delayAdd, dbRules...)
	} else {
		for _, rule := range dbRules {
			directory.Rules[rule.Match] = rule
			d.rules[uint64(rule.ID())] = rule //nolint:gosec
		}
	}

	return nil
}

func toDBRule(rule Rule) *db.Rule {
	return &db.Rule{
		Match:      rule.Match,
		Metadata:   rule.Metadata,
		BackupType: rule.BackupType,
		Override:   rule.Override,
	}
}

// UpdateRule updates the Metadata, BackupType and Override status on a rule.
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

// RemoveRules removes rules on the specified claimed directory.
func (d *Database) RemoveRules(path string, matches ...string) error { //nolint:funlen
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

		return nil
	} else if err := d.rulesDB.RemoveRules(rules...); err != nil {
		return err
	}

	for _, match := range matches {
		delete(d.rules, uint64(directory.Rules[match].ID())) //nolint:gosec
		delete(directory.Rules, match)
	}

	return nil
}

type Directory struct {
	Path       string `json:"-"`
	ClaimedBy  string
	Frequency  uint
	Frozen     bool
	Melt       int64
	ReviewDate int64
	RemoveDate int64
}

// Dirs returns an iterator over all of the claimed directories.
func (d *Database) Dirs() iter.Seq[Directory] {
	return func(yield func(Directory) bool) {
		d.mu.RLock()
		defer d.mu.RUnlock()

		for _, dir := range d.dirs {
			if !yield(toDir(dir.Directory)) {
				return
			}
		}
	}
}

func toDir(dir *db.Directory) Directory {
	return Directory{
		Path:       dir.Path,
		ClaimedBy:  dir.ClaimedBy,
		ReviewDate: dir.ReviewDate,
		RemoveDate: dir.RemoveDate,
		Frequency:  dir.Frequency,
		Frozen:     dir.Frozen,
		Melt:       dir.Melt,
	}
}

// DirRules returns an iterator over all of the rules for a claimed directory.
func (d *Database) DirRules(path string) iter.Seq[Rule] { //nolint:gocognit,gocyclo
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

// Rule returns the rule details for the Rule designated by the supplied ID.
func (d *Database) Rule(id uint64) *Rule {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rule := d.rules[id]
	if rule == nil {
		return nil
	}

	r := ToRule(rule)

	return &r
}

// HasRules returns true if the specified path is a claimed directory with at
// least one rule.
func (d *Database) HasRules(path string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	dir, ok := d.directoryRules[path]
	if !ok {
		return false
	}

	return len(dir.Rules) > 0
}

// RuleTransaction returns a Database on which any AddRules or RemoveRules calls
// will not commit their changes until the Commit method is called.
//
// This allows the changes to be staged while readers see the original,
// unmodified rules.
//
// Only a single such transaction can exist at any one time.
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

// Commit will commit the staged add and remove changes to the database.
func (d *Database) Commit() error { //nolint:gocognit,gocyclo,funlen
	if !d.inTx {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.tx == nil {
		return nil
	}

	if len(d.delayRemove) > 0 {
		if err := d.rulesDB.RemoveRules(d.delayRemove...); err != nil {
			return err
		}

		for _, rule := range d.delayRemove {
			delete(d.dirs[uint64(rule.DirID())].Rules, rule.Match) //nolint:gosec
			delete(d.rules, uint64(rule.ID()))                     //nolint:gosec
		}
	}

	for _, add := range d.delayAdd {
		dr := d.dirs[uint64(add.DirID())] //nolint:gosec
		if dr == nil {
			continue
		}

		dr.Rules[add.Match] = add
		d.rules[uint64(add.ID())] = add //nolint:gosec
	}

	for _, rm := range d.delayRemove {
		dr := d.dirs[uint64(rm.DirID())] //nolint:gosec
		if dr == nil {
			continue
		}

		delete(dr.Rules, rm.Match)
	}

	d.delayAdd = nil
	d.delayRemove = nil

	d.tx.Unlock()
	d.tx = nil

	return nil
}

// Rollback will undo any rule additions and removals that have been staged.
func (d *Database) Rollback() error {
	if !d.inTx {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.tx == nil {
		return nil
	}

	if len(d.delayAdd) > 0 {
		if err := d.rulesDB.RemoveRules(d.delayAdd...); err != nil {
			return err
		}
	}

	d.delayAdd = nil
	d.delayRemove = nil

	d.tx.Unlock()
	d.tx = nil

	return nil
}

// ToRule converts a db.Rule to a rules.Rule.
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
