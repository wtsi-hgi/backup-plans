package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type SQLSource struct {
	db        *sql.DB
	tableName string
}

type SQLiteSource struct {
	*SQLSource
}

type MySQLSource struct {
	*SQLSource
}

const DefaultTableName = "entries"

const createSQLiteTableTmpl = `CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	reporting_name TEXT,
	reporting_root TEXT,
	directory TEXT,
	instruction TEXT CHECK ( instruction IN ('%s', '%s', '%s', '%s') ),
	metadata TEXT,
	keep TEXT,
	skip TEXT,
	requestor TEXT,
	faculty TEXT
)`

const createMySQLTableTmpl = `CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTO_INCREMENT,
	reporting_name TINYTEXT NOT NULL,
	reporting_root MEDIUMTEXT NOT NULL,
	directory MEDIUMTEXT NOT NULL,
	instruction ENUM('%s', '%s', '%s', '%s') NOT NULL,
	metadata MEDIUMTEXT,
	keep MEDIUMTEXT,
	skip MEDIUMTEXT,
	requestor VARCHAR(10) NOT NULL,
	faculty VARCHAR(30) NOT NULL
)`

const (
	getEntryStmt        = getAllStmt + " WHERE id = ?"
	deleteEntryStmt     = "DELETE FROM %s WHERE id = ?"
	deleteReturningStmt = "DELETE FROM %s WHERE id = ? RETURNING *"
	getAllStmt          = `SELECT id, reporting_name, reporting_root, directory, instruction, 
		                   metadata, keep, skip, requestor, faculty FROM %s`
	updateEntryStmt = `UPDATE %s 
					   SET reporting_name = ?, reporting_root = ?, directory = ?, instruction = ?, 
                       metadata = ?, keep = ?, skip = ?, requestor = ?, faculty = ? WHERE id = ?`
	insertEntryStmt = `INSERT INTO %s 
			           (reporting_name, reporting_root, directory, instruction, metadata, keep, skip, requestor, faculty) 
			           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
)

var ErrMissingArgument = errors.New("missing required argument")

// NewSQLiteSource opens a connection to an SQLite database at the given path and stores it internally.
// It also creates a table with the given name if it does not exist.
// You are responsible to close the connection using Close().
func NewSQLiteSource(path string) (SQLiteSource, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return SQLiteSource{}, err
	}

	sq := SQLiteSource{&SQLSource{db: db, tableName: DefaultTableName}}

	return sq, sq.CreateTable()
}

func NewMySQLSourceFromEnv(tableName string) (MySQLSource, error) {
	return NewMySQLSource(
		os.Getenv("MYSQL_HOST"),
		os.Getenv("MYSQL_PORT"),
		os.Getenv("MYSQL_USER"),
		os.Getenv("MYSQL_PASS"),
		os.Getenv("MYSQL_DATABASE"),
		tableName,
	)
}

// NewMySQLSource opens a connection to a MySQL database using given credentials and stores it internally.
// It also creates a table with the given name if it does not exist.
// You are responsible to close the connection using Close().
func NewMySQLSource(host, port, user, password, dbName, tableName string) (MySQLSource, error) {
	var missing []string
	if host == "" {
		missing = append(missing, "host")
	}
	if port == "" {
		missing = append(missing, "port")
	}
	if user == "" {
		missing = append(missing, "user")
	}
	if password == "" {
		missing = append(missing, "password")
	}
	if dbName == "" {
		missing = append(missing, "dbName")
	}
	if len(missing) > 0 {
		return MySQLSource{}, fmt.Errorf("%w: %v\n", ErrMissingArgument, missing)
	}

	address := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, dbName)

	db, err := sql.Open("mysql", address)
	if err != nil {
		return MySQLSource{}, err
	}

	sq := MySQLSource{&SQLSource{db: db, tableName: tableName}}

	tables, err := sq.ShowTables()
	if err != nil {
		return sq, err
	}

	if !slices.Contains(tables, tableName) {
		return sq, sq.CreateTable()
	}

	return sq, nil
}

func (sq SQLSource) Close() error {
	return sq.db.Close()
}

func (sq SQLiteSource) CreateTable() error {
	return sq.createTable(createSQLiteTableTmpl)
}

func (sq SQLSource) createTable(tmpl string) error {
	createTableStmt := fmt.Sprintf(tmpl, sq.tableName, Backup, NoBackup, TempBackup, ManualBackup)
	_, err := sq.db.Exec(createTableStmt)

	return err
}

func (sq MySQLSource) CreateTable() error {
	return sq.createTable(createMySQLTableTmpl)
}

func (sq SQLSource) ReadAll() ([]*Entry, error) {
	rows, err := sq.db.Query(fmt.Sprintf(getAllStmt, sq.tableName))
	if err != nil {
		return nil, err
	}

	defer callAndLogError(rows.Close)

	var entries []*Entry

	for rows.Next() {
		entry, err := sq.scanEntry(rows)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func (sq SQLSource) scanEntry(row scanner) (*Entry, error) {
	var entry Entry

	err := row.Scan(&entry.ID, &entry.ReportingName, &entry.ReportingRoot, &entry.Directory,
		&entry.Instruction, &entry.Metadata, &entry.Match, &entry.Ignore, &entry.Requestor, &entry.Faculty)

	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoEntry
	}

	return &entry, err
}

func (sq SQLSource) GetEntry(id uint16) (*Entry, error) {
	stmt := fmt.Sprintf(getEntryStmt, sq.tableName)

	row := sq.db.QueryRow(stmt, id)

	entry, err := sq.scanEntry(row)

	return entry, err
}

func (sq SQLSource) UpdateEntry(newEntry *Entry) error {
	stmt := fmt.Sprintf(updateEntryStmt, sq.tableName)

	ctx := context.Background()
	r, err := sq.db.ExecContext(ctx, stmt, newEntry.ReportingName, newEntry.ReportingRoot,
		newEntry.Directory, newEntry.Instruction, newEntry.Metadata, newEntry.Match,
		newEntry.Ignore, newEntry.Requestor, newEntry.Faculty, newEntry.ID)

	if err != nil {
		return err
	}

	count, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return ErrNoEntry
	}

	return nil
}

func (sq SQLiteSource) DeleteEntry(id uint16) (*Entry, error) {
	stmt := fmt.Sprintf(deleteReturningStmt, sq.tableName)

	row := sq.db.QueryRow(stmt, id)

	entry, err := sq.scanEntry(row)

	return entry, err
}

func (sq MySQLSource) DeleteEntry(id uint16) (entry *Entry, err error) {
	tx, err := sq.db.Begin()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			callAndLogError(tx.Rollback)
		} else {
			err = tx.Commit()
		}
	}()

	getStmt := fmt.Sprintf(getEntryStmt, sq.tableName)
	row := tx.QueryRow(getStmt, id)

	entry, err = sq.scanEntry(row)
	if err != nil {
		return nil, err
	}

	delStmt := fmt.Sprintf(deleteEntryStmt, sq.tableName)

	_, err = tx.Exec(delStmt, id)
	if err != nil {
		return nil, err
	}

	return entry, err
}

func (sq SQLSource) AddEntry(entry *Entry) error {
	return sq.WriteEntries([]*Entry{entry})
}

func (sq SQLSource) WriteEntries(entries []*Entry) (err error) {
	tx, err := sq.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			callAndLogError(tx.Rollback)
		} else {
			err = tx.Commit()
		}
	}()

	stmt, err := tx.Prepare(fmt.Sprintf(insertEntryStmt, sq.tableName))
	if err != nil {
		return err
	}
	defer callAndLogError(stmt.Close)

	for _, entry := range entries {
		r, err := stmt.Exec(entry.ReportingName, entry.ReportingRoot, entry.Directory,
			entry.Instruction, entry.Metadata, entry.Match, entry.Ignore, entry.Requestor, entry.Faculty)

		if err != nil {
			return err
		}

		id, err := r.LastInsertId()
		if err != nil {
			return err
		}

		entry.ID = uint16(id)
	}

	return err
}

func (sq SQLSource) DropTable() error {
	_, err := sq.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", sq.tableName))
	return err
}

func (sq MySQLSource) ShowTables() ([]string, error) {
	return sq.scanTableNames("SHOW TABLES")
}

func (sq SQLSource) scanTableNames(stmt string) ([]string, error) {
	rows, err := sq.db.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer callAndLogError(rows.Close)

	var tableName string
	var tableNames []string

	for rows.Next() {
		err = rows.Scan(&tableName)
		if err != nil {
			return nil, err
		}

		tableNames = append(tableNames, tableName)
	}

	return tableNames, nil
}

func (sq SQLiteSource) ShowTables() ([]string, error) {
	return sq.scanTableNames("SELECT name FROM sqlite_master WHERE type='table'")
}
