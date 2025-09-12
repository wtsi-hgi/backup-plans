package db

import "time"

type Directory struct {
	id        int64
	Path      string
	ClaimedBy string

	Created, Modified time.Time
}

func (d *DB) CreateDirectory(dir *Directory) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	dir.Created = time.Now().Truncate(time.Second)
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

func (d *DB) UpdateDirectory(dir *Directory) error {
	dir.Modified = time.Now().Truncate(time.Second)

	return d.exec(updateDirectory, dir.ClaimedBy, dir.Modified, dir.id)
}

func (d *DB) RemoveDirectory(dir *Directory) error {
	return d.exec(deleteDirectory, dir.id)
}
