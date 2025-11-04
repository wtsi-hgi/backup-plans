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
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql" //
	_ "modernc.org/sqlite"             //
)

type DBRO struct { //nolint:revive
	db *sql.DB
}

type DB struct {
	DBRO
}

// InitRO connects to a rule database as determined by the given driver and
// connection strings, disallowing any modifications.
func InitRO(driver, connection string) (*DBRO, error) {
	db, err := sql.Open(driver, connection)
	if err != nil {
		return nil, err
	}

	return &DBRO{db: db}, nil
}

// Init connects to a rule database given a connection string.
//
// Eg:
//
//	sqlite:some/path/db.sqlite
//	mysql:user:password@tcp(host:port)/dbname
func Init(connection string) (*DB, error) {
	driver := "sqlite"

	protocol, uri, _ := strings.Cut(connection, ":")
	switch protocol {
	case "sqlite", "sqlite3":
	case "mysql":
		driver = "mysql"
	default:
		return nil, fmt.Errorf("unrecognised db driver: %s", protocol) //nolint:err113
	}

	db, err := sql.Open(driver, uri)
	if err != nil {
		return nil, err
	}

	d := &DB{
		DBRO: DBRO{db: db},
	}

	if err = d.initTables(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *DB) initTables() error {
	for n, table := range tables {
		var exists int

		if err := d.db.QueryRow(tableCheck, tableNames[n]).Scan(&exists); err != nil {
			return err
		}

		if exists != 0 {
			continue
		}

		if _, err := d.db.Exec(table); err != nil { //nolint:noctx
			return err
		}
	}

	return nil
}

func (d *DB) exec(sql string, params ...any) error {
	tx, err := d.db.Begin() //nolint:noctx
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err = tx.Exec(sql, params...); err != nil { //nolint:noctx
		return err
	}

	return tx.Commit()
}

// Close closes the database connection.
func (d *DBRO) Close() error {
	return d.db.Close()
}
