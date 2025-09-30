package sources

import (
	"errors"
	"testing"

	. "github.com/smarty/assertions"
)

func TestParseInstruction(t *testing.T) {
	for k, v := range instructionLookup {
		term, err := ParseInstruction(k)

		if err != nil {
			t.Error(err)
		}

		if ok, err := So(term, ShouldEqual, v); !ok {
			t.Error(err)
		}

		_, err = ParseInstruction("invalid")
		if ok, err := So(errors.Is(err, ErrWrongInstruction), ShouldBeTrue); !ok {
			t.Error(err)
		}
	}
}

func testDataSourceReadAll(t *testing.T, ds DataSource, originalEntries []*Entry) {
	t.Helper()

	entries, err := ds.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := So(entries, ShouldHaveLength, len(originalEntries)); !ok {
		t.Fatal(err)
	}

	for i := range entries {
		if ok, err := So(entries[i], ShouldResemble, originalEntries[i]); !ok {
			t.Error(err)
		}
	}
}

func testDataSourceGetEntry(t *testing.T, ds DataSource, originalEntries []*Entry) {
	t.Helper()

	for _, originalEntry := range originalEntries {
		entry, err := ds.GetEntry(originalEntry.ID)
		if err != nil {
			t.Fatal(err)
		}

		if ok, err := So(entry, ShouldResemble, originalEntry); !ok {
			t.Error(err)
		}
	}

	t.Run("Get non existing entry", func(t *testing.T) {
		_, err := ds.GetEntry(NumTestDataRows + 100)
		if ok, err := So(errors.Is(err, ErrNoEntry), ShouldBeTrue); !ok {
			t.Error(err)
		}
	})
}

func testDataSourceUpdateEntry(t *testing.T, ds DataSource, originalEntries []*Entry) {
	t.Helper()

	newEntry := originalEntries[0]
	newEntry.ReportingName = "test_project_updated"

	testCases := []struct {
		name    string
		entry   *Entry
		wantErr error
	}{
		{"Update existing entry", newEntry, nil},
		{"Update non-existing entry", &Entry{ID: NumTestDataRows + 100}, ErrNoEntry},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := ds.UpdateEntry(tt.entry)
			if !errors.Is(err, tt.wantErr) {
				t.Fatal(err)
			}

			if tt.wantErr != nil {
				return
			}

			entries, err := ds.ReadAll()
			if err != nil {
				t.Fatal(err)
			}

			if ok, err := So(entries, ShouldHaveLength, len(originalEntries)); !ok {
				t.Fatal(err)
			}

			if ok, err := So(entries[0], ShouldResemble, tt.entry); !ok {
				t.Error(err)
			}
		})
	}

}

func testDataSourceDeleteEntry(t *testing.T, ds DataSource, originalEntry *Entry, idToDelete uint16, expectedErr error) {
	entry, err := ds.DeleteEntry(idToDelete)
	if !errors.Is(err, expectedErr) {
		t.Fatal(err)
	}

	if expectedErr != nil {
		return
	}

	if ok, err := So(entry, ShouldResemble, originalEntry); !ok {
		t.Error(err)
	}

	entriesAfter, err := ds.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := So(entriesAfter, ShouldHaveLength, NumTestDataRows-1); !ok {
		t.Error(err)
	}

	for _, e := range entriesAfter {
		if e.ID == idToDelete {
			t.Errorf("Deleted entry still present: %+v", e)
		}
	}
}

func testDataSourceAddEntry(t *testing.T, ds DataSource, originalEntries []*Entry) {
	newEntry := originalEntries[0]
	newEntry.ReportingName = "test_project_new"

	err := ds.AddEntry(newEntry)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := ds.ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := So(entries, ShouldHaveLength, len(originalEntries)+1); !ok {
		t.Fatal(err)
	}

	newEntry.ID = originalEntries[len(originalEntries)-1].ID + 1

	if ok, err := So(entries[NumTestDataRows], ShouldResemble, newEntry); !ok {
		t.Error(err)
	}
}
