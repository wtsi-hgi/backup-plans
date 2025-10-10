package db

import "time"

// Directory represents a claimed directory that may be given rules.
type Directory struct {
	id        int64
	Path      string
	ClaimedBy string

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
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	dir.Created = time.Now().Unix()
	dir.Modified = dir.Created

	res, err := tx.Exec(createDirectory, dir.Path, dir.ClaimedBy, dir.Created, dir.Modified)
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

	if err := scanner.Scan(
		&dir.id,
		&dir.Path,
		&dir.ClaimedBy,
		&dir.Created,
		&dir.Modified,
	); err != nil {
		return nil, err
	}

	return dir, nil
}

// UpdateDirectory will update the data stored for the given Directory.
func (d *DB) UpdateDirectory(dir *Directory) error {
	dir.Modified = time.Now().Unix()

	return d.exec(updateDirectory, dir.ClaimedBy, dir.Modified, dir.id)
}

// RemoveDirectory will remove the given Directory from the database.
func (d *DB) RemoveDirectory(dir *Directory) error {
	return d.exec(deleteDirectory, dir.id)
}
