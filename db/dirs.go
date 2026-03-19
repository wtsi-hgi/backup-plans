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

// Directory represents a claimed directory that may be given rules.
type Directory struct {
	id         int64
	Path       string
	ClaimedBy  string
	ReviewDate int64
	RemoveDate int64
	Frequency  uint
	Frozen     bool
	Unfreeze   time.Time

	Created, Modified int64
}

// ID returns the in SQL ID for the Directory.
func (d *Directory) ID() int64 {
	if d == nil {
		return 0
	}

	return d.id
}

// CreateDirectory adds the given Directory structure to the database.
func (d *DB) CreateDirectory(dir *Directory) error {
	tx, err := d.db.Begin() //nolint:noctx
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	dir.Created = time.Now().Unix()
	dir.Modified = dir.Created

	res, err := tx.Exec(createDirectory, dir.Path, dir.ClaimedBy, dir.Frequency, //nolint:noctx
		dir.Frozen, dir.ReviewDate, dir.RemoveDate, dir.Created, dir.Modified)
	if err != nil {
		return err
	}

	if dir.id, err = res.LastInsertId(); err != nil {
		return err
	}

	return tx.Commit()
}

// ReadDirectories allows iteration over the Directories stored in the database.
func (d *DBRO) ReadDirectories() *IterErr[*Directory] {
	return iterRows(d, scanDirectory, selectAllDirectories)
}

func scanDirectory(scanner scanner) (*Directory, error) {
	dir := new(Directory)

	var unfreeze int64

	if err := scanner.Scan(
		&dir.id,
		&dir.Path,
		&dir.ClaimedBy,
		&dir.Frequency,
		&dir.Frozen,
		&dir.ReviewDate,
		&dir.RemoveDate,
		&unfreeze,
		&dir.Created,
		&dir.Modified,
	); err != nil {
		return nil, err
	}

	dir.Unfreeze = time.Unix(unfreeze, 0)

	return dir, nil
}

// UpdateDirectory will update the data stored for the given Directory.
func (d *DB) UpdateDirectory(dir *Directory) error {
	dir.Modified = time.Now().Unix()

	return d.exec(updateDirectory, dir.ClaimedBy, dir.Modified, dir.Frequency,
		dir.Frozen, dir.ReviewDate, dir.RemoveDate, dir.id)
}

// RemoveDirectory will remove the given Directory from the database.
func (d *DB) RemoveDirectory(dir *Directory) error {
	return d.exec(deleteDirectory, dir.id)
}

func (d *DB) Thaw(dir *Directory) error {
	return d.setDirFreezeTime(dir, time.Now().Truncate(time.Second))
}

func (d *DB) setDirFreezeTime(dir *Directory, t time.Time) error {
	if err := d.exec(setDirectoryFreeze, t.Unix(), dir.id); err != nil {
		return err
	}

	dir.Unfreeze = t

	return nil
}

func (d *DB) Refreeze(dir *Directory) error {
	return d.setDirFreezeTime(dir, time.Unix(0, 0))
}
