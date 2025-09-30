package sources

import (
	"errors"
	"log"
	"path/filepath"
	"testing"

	. "github.com/smarty/assertions"
)

var sqlTestCases = []struct {
	name string
	src  func(t *testing.T) ([]*Entry, DataSource)
}{
	{"SQLite", setupSQLiteSourceForTest},
	{"MySQL", setupMySQLSourceForTest},
}

func TestSQLSource_ReadAll(t *testing.T) {
	for _, tt := range sqlTestCases {
		t.Run(tt.name, func(t *testing.T) {
			entries, sq := tt.src(t)

			testDataSourceReadAll(t, sq, entries)
		})
	}
}

func TestSQLSource_GetEntry(t *testing.T) {
	for _, tt := range sqlTestCases {
		t.Run(tt.name, func(t *testing.T) {
			entries, sq := tt.src(t)

			testDataSourceGetEntry(t, sq, entries)
		})
	}
}

func TestSQLSource_UpdateEntry(t *testing.T) {
	for _, tt := range sqlTestCases {
		t.Run(tt.name, func(t *testing.T) {
			entries, sq := tt.src(t)

			testDataSourceUpdateEntry(t, sq, entries)
		})
	}
}

func TestSQLSource_DeleteEntry(t *testing.T) {
	testCases := []struct {
		name    string
		entryID uint16
		wantErr error
	}{
		{"Delete first entry", 1, nil},
		{"Delete middle entry", max(1, NumTestDataRows-1), nil},
		{"Delete last entry", NumTestDataRows, nil},
		{"Delete non-existing entry", NumTestDataRows + 100, ErrNoEntry},
	}

	for _, sqlTest := range sqlTestCases {
		t.Run(sqlTest.name, func(t *testing.T) {

			for _, tt := range testCases {
				t.Run(tt.name, func(t *testing.T) {
					entries, sq := sqlTest.src(t)

					var e *Entry
					if tt.entryID > NumTestDataRows {
						e = nil
					} else {
						e = entries[tt.entryID-1]
					}

					testDataSourceDeleteEntry(t, sq, e, tt.entryID, tt.wantErr)
				})
			}
		})
	}
}

func TestSQLSource_AddEntry(t *testing.T) {
	for _, sqlTest := range sqlTestCases {
		t.Run(sqlTest.name, func(t *testing.T) {
			entries, sq := sqlTest.src(t)

			testDataSourceAddEntry(t, sq, entries)
		})
	}
}

func TestSQLiteSource_WriteEntries(t *testing.T) {
	entries := createTestEntries(t)

	dbPath := filepath.Join(t.TempDir(), "test.db")

	sq, err := NewSQLiteSource(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer callAndLogTestError(t, sq.Close)

	err = sq.WriteEntries(entries)
	if err != nil {
		t.Fatal(err)
	}

	for i, entry := range entries {
		ok, err := So(entry.ID, ShouldEqual, uint16(i+1))
		if !ok {
			t.Error(err)
		}
	}
}

func TestMySQLSource_WriteEntries(t *testing.T) {
	tableName := "entries_test"

	sq, err := NewMySQLSourceFromEnv(tableName)
	if err != nil {
		if errors.Is(err, ErrMissingArgument) {
			t.Skip("Skipping MySQL test because MySQL host, port, user, pass, or database is not set.")
		}

		t.Fatal(err)
	}
	defer callAndLogTestError(t, sq.Close)
	defer cleanupMySQL(t, sq)

	entries := createTestEntries(t)

	err = sq.WriteEntries(entries)
	if err != nil {
		t.Fatal(err)
	}

	for i, entry := range entries {
		ok, err := So(entry.ID, ShouldEqual, uint16(i+1))
		if !ok {
			t.Error(err)
		}
	}
}

func TestNewSQLiteSource(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "test.db")

	sq, err := NewSQLiteSource(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer callAndLogTestError(t, sq.Close)

	tableNames, err := sq.ShowTables()
	if err != nil {
		log.Fatal(err)
	}

	if ok, err := So(tableNames, ShouldContain, DefaultTableName); !ok {
		log.Fatal(err)
	}
}

func TestNewMySQLSource(t *testing.T) {
	tableName := "test_create_table"

	sq, err := NewMySQLSourceFromEnv(tableName)
	if err != nil {
		if errors.Is(err, ErrMissingArgument) {
			t.Skip("Skipping MySQL test because MySQL host, port, user, pass, or database is not set.")
		}

		t.Fatal(err)
	}
	defer callAndLogTestError(t, sq.Close)

	defer cleanupMySQL(t, sq)

	tableNames, err := sq.ShowTables()
	if err != nil {
		log.Fatal(err)
	}

	if ok, err := So(tableNames, ShouldContain, tableName); !ok {
		log.Fatal(err)
	}
}

func setupSQLiteSourceForTest(t *testing.T) ([]*Entry, DataSource) {
	t.Helper()

	entries, sq := CreateTestSQLiteTable(t)

	cleanup := func() {
		callAndLogTestError(t, sq.Close)
	}

	t.Cleanup(cleanup)

	return entries, sq
}

// createTestMySQLTable initialises a connection to a MySQL database, creates a table, inserts test entries, and
// returns them and MySQL source. You should close the database connection with sq.Close() once it no longer needed.
func createTestMySQLTable(t *testing.T) ([]*Entry, MySQLSource, string) {
	t.Helper()

	entries := createTestEntries(t)
	for _, entry := range entries {
		entry.ID += 1
	}

	tableName := "entries_test"

	sq, err := NewMySQLSourceFromEnv(tableName)
	if err != nil {
		if errors.Is(err, ErrMissingArgument) {
			t.Skip("Skipping MySQL test because MySQL host, port, user, pass, or database is not set.")
		}

		t.Fatal(err)
	}

	err = sq.WriteEntries(entries)
	if err != nil {
		t.Fatal(err)
	}

	return entries, sq, tableName
}

func setupMySQLSourceForTest(t *testing.T) ([]*Entry, DataSource) {
	t.Helper()

	entries, sq, _ := createTestMySQLTable(t)

	cleanup := func() {
		cleanupMySQL(t, sq)
	}

	t.Cleanup(cleanup)

	return entries, sq
}

func cleanupMySQL(t *testing.T, sq MySQLSource) {
	t.Helper()

	callAndLogTestError(t, sq.DropTable)
	callAndLogTestError(t, sq.Close)
}

func callAndLogTestError(t *testing.T, f func() error) {
	t.Helper()

	err := f()
	if err != nil {
		t.Log(err)
	}
}
