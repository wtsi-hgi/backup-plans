/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Authors:
 *	- Sky Haines <sh55@sanger.ac.uk>
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

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/pgzip"
	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/treegen"
	"github.com/wtsi-hgi/wrstat-ui/stats"
	"github.com/wtsi-hgi/wrstat-ui/summary"
)

var ErrArgs = errors.New("requires path to stats.gz file and output tree location")

// dbCmd represents the db command.
var dbCmd = &cobra.Command{
	Use:   "db <stats.gz> <tree.db>",
	Short: "Create tree database using summarise",
	Long: `Create tree database using summarise.

Provide the path to a wrstat stats.gz file and the path to your desired tree
database file.
`,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 2 {
			return ErrArgs
		}

		sf, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("error opening stats file: %w", err)
		}

		defer sf.Close()

		var r io.Reader

		if strings.HasSuffix(args[0], ".gz") {
			if r, err = pgzip.NewReader(sf); err != nil {
				return fmt.Errorf("error decompressing stats file: %w", err)
			}
		} else {
			r = sf
		}

		s := summary.NewSummariser(stats.NewStatsParser(r))

		f, err := os.Create(args[1])
		if err != nil {
			return fmt.Errorf("error creating output tree file: %w", err)
		}

		b := bufio.NewWriter(f)

		s.AddDirectoryOperation(treegen.NewTree(b))

		if err := s.Summarise(); err != nil {
			return fmt.Errorf("error creating tree db: %w", err)
		}

		if err := b.Flush(); err != nil {
			return fmt.Errorf("error flushing tree db: %w", err)
		}

		return f.Close()
	},
}

func init() {
	RootCmd.AddCommand(dbCmd)
}
