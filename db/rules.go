package db

import (
	"time"
)

// BackupType is an 'enum' representing the known backup types.
type BackupType uint8

const (
	BackupNone BackupType = iota
	BackupTemp
	BackupIBackup
	BackupManual
)

// Rule represents a defined rule.
type Rule struct {
	id          int64
	directoryID int64
	BackupType  BackupType
	Metadata    string
	ReviewDate  int64
	RemoveDate  int64
	Match       string
	Frequency   uint

	Created, Modified int64
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

// CreateDirectoryRule stores defines the given rule for the given directory.
func (d *DB) CreateDirectoryRule(dir *Directory, rule *Rule) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	rule.Created = time.Now().Unix()
	rule.Modified = rule.Created

	res, err := tx.Exec(createRule, dir.id, rule.BackupType, rule.Metadata, rule.ReviewDate, rule.RemoveDate, rule.Match, rule.Frequency, rule.Created, rule.Modified)
	if err != nil {
		return err
	}

	if rule.id, err = res.LastInsertId(); err != nil {
		return err
	}

	rule.directoryID = dir.id

	return tx.Commit()
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
		&rule.ReviewDate,
		&rule.RemoveDate,
		&rule.Match,
		&rule.Frequency,
		&rule.Created,
		&rule.Modified,
	); err != nil {
		return nil, err
	}

	return rule, nil
}

// UpdateDirectory will update the data stored for the given Rule.
func (d *DB) UpdateRule(rule *Rule) error {
	rule.Modified = time.Now().Unix()

	return d.exec(updateRule, rule.BackupType, rule.Metadata, rule.ReviewDate, rule.RemoveDate, rule.Match, rule.Frequency, rule.Modified, rule.id)
}

// RemoveRule will remove the given Rule from the database.
func (d *DB) RemoveRule(rule *Rule) error {
	return d.exec(deleteRule, rule.id)
}
