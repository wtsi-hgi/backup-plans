package sources

import (
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/smarty/assertions"
)

func TestCSVSource_ReadAll(t *testing.T) {
	entries, testPath := CreateTestCSV(t)

	csvSource := CSVSource{Path: testPath}

	testDataSourceReadAll(t, csvSource, entries)
}

func TestCSVSource_GetEntry(t *testing.T) {
	entries, testPath := CreateTestCSV(t)

	csvSource := CSVSource{Path: testPath}

	testDataSourceGetEntry(t, csvSource, entries)
}

func TestCSVSource_UpdateEntry(t *testing.T) {
	entries, testPath := CreateTestCSV(t)

	csvSource := CSVSource{Path: testPath}

	testDataSourceUpdateEntry(t, csvSource, entries)
}

func TestCSVSource_DeleteEntry(t *testing.T) {
	testCases := []struct {
		name    string
		entryID uint16
		wantErr error
	}{
		{"Delete first entry", 0, nil},
		{"Delete middle entry", max(0, NumTestDataRows-2), nil},
		{"Delete last entry", NumTestDataRows - 1, nil},
		{"Delete non-existing entry", NumTestDataRows + 100, ErrNoEntry},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			entries, testPath := CreateTestCSV(t)

			csvSource := CSVSource{Path: testPath}

			var e *Entry
			if tt.entryID > NumTestDataRows {
				e = nil
			} else {
				e = entries[tt.entryID]
			}

			testDataSourceDeleteEntry(t, csvSource, e, tt.entryID, tt.wantErr)
		})
	}
}

func TestCSVSource_AddEntry(t *testing.T) {
	entries, filePath := CreateTestCSV(t)

	csvSource := CSVSource{Path: filePath}

	testDataSourceAddEntry(t, csvSource, entries)
}

func TestCSVSource_WriteEntries(t *testing.T) {
	entries, _ := CreateTestCSV(t)

	filePath := filepath.Join(t.TempDir(), "test.csv")
	csvSource := CSVSource{Path: filePath}

	err := csvSource.writeEntries(entries)
	if err != nil {
		t.Fatal(err)
	}

	newEntries, err := csvSource.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := So(newEntries, ShouldHaveLength, NumTestDataRows); !ok {
		t.Fatal(err)
	}

	if ok, err := So(newEntries, ShouldResemble, entries); !ok {
		t.Error(err)
	}
}

func TestGetNextID(t *testing.T) {
	tests := []struct {
		entries    []*Entry
		expectedID uint16
	}{
		{
			entries:    []*Entry{{ID: 0}, {ID: 1}, {ID: 2}},
			expectedID: 3,
		},
		{
			entries:    []*Entry{{ID: 0}, {ID: 2}, {ID: 3}},
			expectedID: 1,
		},
		{
			entries:    []*Entry{{ID: 1}, {ID: 5}, {ID: 6}},
			expectedID: 0,
		},
	}

	csvSource := CSVSource{}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Expect %d", test.expectedID), func(t *testing.T) {
			id := csvSource.getNextID(test.entries)
			if ok, err := So(id, ShouldEqual, test.expectedID); !ok {
				t.Error(err)
			}
		})
	}
}
