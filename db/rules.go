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

package db

import (
	"time"
)

// BackupType is an 'enum' representing the known backup types.
type BackupType uint8

const (
	BackupNone BackupType = iota
	BackupIBackup
	BackupManualIBackup
	BackupManualGit
	BackupManualUnchecked
	BackupManualPrefect
	BackupManualNFS
)

// Rule represents a defined rule.
type Rule struct {
	id          int64
	directoryID int64
	BackupType  BackupType
	Metadata    string // requester:name for manual
	Match       string
	Override    bool

	Created, Modified int64
}

// IsManual returns whether the specified ID corresponds to a manual backup type.
func IsManual(bt BackupType) bool {
	return bt > 1
}

// ID returns the in SQL ID for the Rule.
func (r *Rule) ID() int64 {
	if r == nil {
		return 0
	}

	return r.id
}

// DirID returns the in SQL ID for the Directory the rule is attached to.
func (r *Rule) DirID() int64 {
	if r == nil {
		return 0
	}

	return r.directoryID
}

// CreateDirectoryRule defines the given rule(s) for the given directory.
func (d *DB) CreateDirectoryRule(dir *Directory, rules ...*Rule) error { //nolint:funlen
	tx, err := d.db.Begin() //nolint:noctx
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().Unix()

	for _, rule := range rules {
		rule.Created = now
		rule.Modified = now

		res, err := tx.Exec( //nolint:noctx
			createRule,
			dir.id,
			rule.BackupType,
			rule.Metadata,
			rule.Match,
			rule.Override,
			rule.Created,
			rule.Modified,
		)
		if err != nil {
			return err
		}

		if rule.id, err = res.LastInsertId(); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	for _, rule := range rules {
		rule.directoryID = dir.id
	}

	return nil
}

// ReadRules allows iteration over the Rules stored in the database.
func (d *DBRO) ReadRules() *IterErr[*Rule] {
	return iterRows(d, scanRule, selectAllRules)
}

func scanRule(scanner scanner) (*Rule, error) {
	rule := new(Rule)

	if err := scanner.Scan(
		&rule.id,
		&rule.directoryID,
		&rule.BackupType,
		&rule.Metadata,
		&rule.Match,
		&rule.Override,
		&rule.Created,
		&rule.Modified,
	); err != nil {
		return nil, err
	}

	return rule, nil
}

// UpdateRule will update the data stored for the given Rule(s).
func (d *DB) UpdateRule(rules ...*Rule) error {
	tx, err := d.db.Begin() //nolint:noctx
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().Unix()

	for _, rule := range rules {
		rule.Modified = now

		if _, err := tx.Exec( //nolint:noctx
			updateRule,
			rule.BackupType,
			rule.Metadata,
			rule.Match,
			rule.Modified,
			rule.id,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// RemoveRule will remove the given Rule from the database.
func (d *DB) RemoveRule(rule *Rule) error {
	return d.exec(deleteRule, rule.id)
}
