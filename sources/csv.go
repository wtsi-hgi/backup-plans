package sources

import (
	"os"

	"github.com/gocarina/gocsv"
)

type CSVSource struct {
	Path string
}

func (c CSVSource) ReadAll() ([]*Entry, error) {
	in, err := os.Open(c.Path)
	if err != nil {
		return nil, err
	}

	defer callAndLogError(in.Close)

	var entries []*Entry

	err = gocsv.UnmarshalFile(in, &entries)

	return entries, err
}

func (c CSVSource) GetEntry(id uint16) (*Entry, error) {
	entries, err := c.ReadAll()
	if err != nil {
		return nil, err
	}

	entry, _, err := getMatchingEntryWithID(id, entries)

	return entry, err
}

func getMatchingEntryWithID(id uint16, entries []*Entry) (*Entry, int, error) {
	for i, entry := range entries {
		if entry.ID == id {
			return entry, i, nil
		}
	}

	return nil, 0, ErrNoEntry
}

func (c CSVSource) UpdateEntry(newEntry *Entry) error {
	entries, err := c.ReadAll()
	if err != nil {
		return err
	}

	_, index, err := getMatchingEntryWithID(newEntry.ID, entries)
	if err != nil {
		return err
	}

	entries[index] = newEntry

	return c.writeEntries(entries)
}

func (c CSVSource) writeEntries(entries []*Entry) error {
	out, err := os.Create(c.Path)
	if err != nil {
		return err
	}

	defer callAndLogError(out.Close)

	return gocsv.MarshalFile(&entries, out)
}

func (c CSVSource) DeleteEntry(id uint16) (*Entry, error) {
	entries, err := c.ReadAll()
	if err != nil {
		return nil, err
	}

	entry, index, err := getMatchingEntryWithID(id, entries)
	if err != nil {
		return nil, err
	}

	entries = append(entries[:index], entries[index+1:]...)

	return entry, c.writeEntries(entries)
}

func (c CSVSource) AddEntry(newEntry *Entry) error {
	entries, err := c.ReadAll()
	if err != nil {
		return err
	}

	newEntry.ID = c.getNextID(entries)

	entries = append(entries, newEntry)

	return c.writeEntries(entries)
}

func (c CSVSource) getNextID(entries []*Entry) uint16 {
	used := make(map[uint16]struct{}, len(entries))
	for _, entry := range entries {
		used[entry.ID] = struct{}{}
	}

	// Find gaps
	for i := range uint16(len(used)) {
		_, found := used[i]
		if !found {
			return i
		}
	}

	return uint16(len(used))
}
