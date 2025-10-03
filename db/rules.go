package db

import (
	"time"
)

type BackupType uint8

const (
	BackupNone BackupType = iota
	BackupTemp
	BackupIBackup
	BackupManual
)

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

func (r *Rule) ID() int64 {
	if r == nil {
		return 0
	}

	return r.id
}

func (r *Rule) DirID() int64 {
	if r == nil {
		return 0
	}

	return r.directoryID
}

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

func (d *DB) UpdateRule(rule *Rule) error {
	rule.Modified = time.Now().Unix()

	return d.exec(updateRule, rule.BackupType, rule.Metadata, rule.ReviewDate, rule.RemoveDate, rule.Match, rule.Frequency, rule.Modified, rule.id)
}

func (d *DB) RemoveRule(rule *Rule) error {
	return d.exec(deleteRule, rule.id)
}
