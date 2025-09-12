package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/pgzip"
	"github.com/wtsi-hgi/backup-plans/dbgen"
	"github.com/wtsi-hgi/wrstat-ui/stats"
	"github.com/wtsi-hgi/wrstat-ui/summary"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return ErrArgs
	}

	sf, err := os.Open(os.Args[1])
	if err != nil {
		return fmt.Errorf("error opening stats file: %w", err)
	}

	defer sf.Close()

	var r io.Reader

	if strings.HasSuffix(os.Args[1], ".gz") {
		if r, err = pgzip.NewReader(sf); err != nil {
			return fmt.Errorf("error decompressing stats file: %w", err)
		}
	} else {
		r = sf
	}

	s := summary.NewSummariser(stats.NewStatsParser(r))

	f, err := os.Create(os.Args[2])
	if err != nil {
		return fmt.Errorf("error creating output tree file: %w", err)
	}

	b := bufio.NewWriter(f)

	s.AddDirectoryOperation(dbgen.NewTree(b))

	if err := s.Summarise(); err != nil {
		return fmt.Errorf("error creating tree db: %w", err)
	}

	if err := b.Flush(); err != nil {
		return fmt.Errorf("error flushing tree db: %w", err)
	}

	return f.Close()
}

var ErrArgs = errors.New("requires path to stats.gz file and output tree location")
