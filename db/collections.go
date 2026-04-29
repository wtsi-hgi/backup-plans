/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Sky Haines <sh55@sanger.ac.uk>
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

import "time"

// Collection represents a collection of rules, which can be applied to directories as a single unit.
type Collection struct {
	id          int64
	Name        string
	Description string

	Created, Modified int64
}

// CollectionRule represents a rule which is part of a collection.
// It is not applied to a directory and is merely a template to be applied to one.
type CollectionRule struct {
	id           int64
	collectionID int64
	BackupType   BackupType
	Metadata     string
	Match        string
	Override     bool

	Created, Modified int64
}

// CreateDirectoryRule creates a new collection with the given name and description.
func (d *DB) CreateCollection(c *Collection) error {
	tx, err := d.db.Begin() //nolint:noctx
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().Unix()

	c.Created = now
	c.Modified = now

	res, err := tx.Exec( //nolint:noctx
		createCollection,
		c.Name,
		c.Description,
		c.Created,
		c.Modified,
	)
	if err != nil {
		return err
	}

	if c.id, err = res.LastInsertId(); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// ID returns the in SQL ID for the Collection.
func (c *Collection) ID() int64 {
	if c == nil {
		return 0
	}

	return c.id
}

// ReadCollections allows iteration over the Collections stored in the database.
func (d *DBRO) ReadCollections() *IterErr[*Collection] {
	return iterRows(d, scanCollection, string(selectAllCollections))
}

func scanCollection(scanner scanner) (*Collection, error) {
	c := new(Collection)

	if err := scanner.Scan(
		&c.id,
		&c.Name,
		&c.Description,
		&c.Created,
		&c.Modified,
	); err != nil {
		return nil, err
	}

	return c, nil
}

// UpdateCollection will update the data stored for the given Collection.
func (d *DB) UpdateCollection(c *Collection) error {
	c.Modified = time.Now().Unix()

	return d.exec(updateCollection, c.Name, c.Description, c.Modified, c.id)
}

// RemoveCollection will remove the given collection and all its rules from the database.
func (d *DB) RemoveCollection(collection *Collection) error {
	return d.exec(deleteCollection, collection.id)
}

// CreateCollectionRule defines the given rule(s) for the given collection.
func (d *DB) CreateCollectionRule(c *Collection, rules ...*CollectionRule) error { //nolint:funlen
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
			createCollectionRule,
			c.id,
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

	return nil
}

// ID returns the in SQL ID for the CollectionRule.
func (c *CollectionRule) ID() int64 {
	if c == nil {
		return 0
	}

	return c.id
}

// CollectionID returns the SQL collection ID that the collection rule belongs to.
func (c *CollectionRule) CollectionID() int64 {
	if c == nil {
		return 0
	}

	return c.collectionID
}

// ReadCollectionRules allows iteration over the CollectionRules stored in the database.
func (d *DBRO) ReadCollectionRules() *IterErr[*CollectionRule] {
	return iterRows(d, scanCollectionRule, string(selectAllCollectionRules))
}

func scanCollectionRule(scanner scanner) (*CollectionRule, error) {
	r := new(CollectionRule)

	if err := scanner.Scan(
		&r.id,
		&r.collectionID,
		&r.BackupType,
		&r.Metadata,
		&r.Match,
		&r.Override,
		&r.Created,
		&r.Modified,
	); err != nil {
		return nil, err
	}

	return r, nil
}

// UpdateCollectionRule will update the data stored for the given Directory.
func (d *DB) UpdateCollectionRule(c *CollectionRule) error {
	c.Modified = time.Now().Unix()

	return d.exec(updateCollectionRule, c.BackupType, c.Metadata, c.Match, c.Override, c.Modified, c.id)
}

// RemoveCollectionRule will remove the given rule from its collection.
func (d *DB) RemoveCollectionRule(r *CollectionRule) error {
	return d.exec(deleteCollectionRule, r.ID())
}
