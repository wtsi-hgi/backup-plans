//go:build test

package sources

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gocarina/gocsv"
)

const NumTestDataRows = 4

func createTestEntries(t *testing.T) []*Entry {
	t.Helper()

	if NumTestDataRows < len(instructionLookup) {
		t.Fatalf("Invalid test setup: NumTestDataRows [%d] must be >= the number of instructions [%d]",
			NumTestDataRows, len(instructionLookup))
	}

	instructions := make([]Instruction, 0, len(instructionLookup))
	for _, v := range instructionLookup {
		instructions = append(instructions, v)
	}

	baseEntry := Entry{
		ReportingName: "",
		ReportingRoot: "/some/path/to/project/dir",
		Directory:     "/some/path/to/project/dir/input",
		Instruction:   instructions[0],
		Metadata:      "",
		Requestor:     "user",
		Faculty:       "group",
	}

	entries := make([]*Entry, NumTestDataRows)
	for i := range NumTestDataRows {
		newEntry := baseEntry

		newEntry.ID = uint16(i)
		newEntry.ReportingName = fmt.Sprintf("test_project_%d", i)
		newEntry.Instruction = instructions[i%len(instructions)]

		if newEntry.Instruction == ManualBackup {
			newEntry.Metadata = "aSetName"
		}

		entries[i] = &newEntry
	}

	return entries
}

func CreateTestCSV(t *testing.T) ([]*Entry, string) {
	t.Helper()

	entries := createTestEntries(t)

	file, err := os.CreateTemp(t.TempDir(), "*.csv")
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = file.Close()
		if err != nil {
			t.Log(err)
		}
	}()

	err = gocsv.MarshalFile(&entries, file)
	if err != nil {
		t.Fatal(err)
	}

	return entries, file.Name()
}

// CreateTestSQLiteTable initialises a test SQLite database, creates a table, inserts test entries, and returns them and
// SQLite source. You should close the database connection with sq.Close() once it no longer needed.
func CreateTestSQLiteTable(t *testing.T) ([]*Entry, SQLiteSource) {
	t.Helper()

	entries := createTestEntries(t)
	for _, entry := range entries {
		entry.ID++
	}

	dbFile := filepath.Join(t.TempDir(), "test.db")

	sq, err := NewSQLiteSource(dbFile)
	if err != nil {
		t.Fatal(err)
	}

	err = sq.WriteEntries(entries)
	if err != nil {
		t.Fatal(err)
	}

	return entries, sq
}
