package sources

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

type DataSource interface {
	ReadAll() ([]*Entry, error)
	GetEntry(id uint16) (*Entry, error)
	UpdateEntry(newEntry *Entry) error
	DeleteEntry(id uint16) (*Entry, error)
	AddEntry(entry *Entry) error
}

type Instruction string

const (
	Backup       Instruction = "backup"
	NoBackup     Instruction = "nobackup"
	TempBackup   Instruction = "tempbackup"
	ManualBackup Instruction = "manual backup"
)

var instructionLookup = map[string]Instruction{
	string(Backup):       Backup,
	string(NoBackup):     NoBackup,
	string(TempBackup):   TempBackup,
	string(ManualBackup): ManualBackup,
}

// ParseInstruction parses a string into a valid Instruction.
func ParseInstruction(s string) (Instruction, error) {
	normalized := strings.TrimSpace(s)
	if v, ok := instructionLookup[normalized]; ok {
		return v, nil
	}
	return "", fmt.Errorf("%w: %s", ErrWrongInstruction, s)
}

type Entry struct {
	ReportingName string      `csv:"reporting_name"`
	ReportingRoot string      `csv:"reporting_root"`
	Directory     string      `csv:"directory"`
	Instruction   Instruction `csv:"instruction"`
	Metadata      string      `csv:"metadata"`
	Match         string      `csv:"match"`
	Ignore        string      `csv:"ignore"`
	Requestor     string      `csv:"requestor"`
	Faculty       string      `csv:"faculty"`
	ID            uint16      `csv:"id"`
	Failures      int         `csv:"failures"`
}

var ErrNoEntry = errors.New("entry does not exist")
var ErrWrongInstruction = errors.New("wrong instruction")

func callAndLogError(f func() error) {
	err := f()
	if err != nil {
		slog.Error(err.Error())
	}
}
