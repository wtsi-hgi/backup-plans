/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Authors:
 *	- Sky Haines <sh55@sanger.ac.uk>
 *  - Michael Woolnough <mw31@sanger.ac.uk>
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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/pgzip"
	"github.com/kuleuven/iron"
	"github.com/kuleuven/iron/cmd/iron/cli"
	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/config"
	"github.com/wtsi-hgi/backup-plans/treegen"
	"github.com/wtsi-hgi/ibackup/cmd"
	"github.com/wtsi-hgi/ibackup/transformer"
	"github.com/wtsi-hgi/wrstat-ui/stats"
	"github.com/wtsi-hgi/wrstat-ui/summary"
	"vimagination.zapto.org/tree"
)

var ErrArgs = errors.New("requires path to stats.gz file and output tree location")

// treeDBLocalCmd represents the db command.
var treeDBLocalCmd = &cobra.Command{
	Use:   "local <stats.gz> <tree.db>",
	Short: "Create tree database using summarise",
	Long: `Create tree database using summarise.

Provide the path to a wrstat stats.gz file and the path to your desired tree
database file.
`,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 2 { //nolint:mnd
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

var (
	ibackupConfig, irodsEnv string

	ErrMissingOutput = errors.New("missing backuptree db output file")
)

// treeDBRemoteCmd represents the backupdb command.
var treeDBRemoteCmd = &cobra.Command{
	Use:   "remote <backuptree.db>",
	Short: "Create tree database from files backed up to iRODs via iBackup.",
	Long: `Create tree database from files backed up in iRODS via iBackup FOFN server.

Provide the path to the output location for the tree of backed up
files/collections.

--config should be the location of a Yaml config file, which should have the
following structure:

Collections:
  /collection/1/: transformer_1
  /collection/2/: transformer_2

With the transformers corresponding either to prefix transformers or to those
specified in the ibackup config file, the location of which should be specified
with either the --ibackup flag or the IBACKUP_CONFIG env var.

In addition, the irods environmental file should be specified either with the
--irods flag or the IRODS_ENVIRONMENT_FILE env var.
`,
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) != 1 {
			return ErrMissingOutput
		}

		config, err := config.Parse(configPath)
		if err != nil {
			return fmt.Errorf("failed to process config file: %w", err)
		}

		err = cmd.LoadConfig(ibackupConfig)
		if err != nil {
			return fmt.Errorf("error loading ibackup config: %w", err)
		}

		collections, err := getCollectionTransformers(config)
		if err != nil {
			return err
		}

		env, err := parseIRODSEnvFile(irodsEnv)
		if err != nil {
			return fmt.Errorf("error loading ibackup config: %w", err)
		}

		f, err := os.Create(args[0])
		if err != nil {
			return fmt.Errorf("error creating output tree file: %w", err)
		}

		defer func() {
			if errr := f.Close(); err == nil {
				err = errr
			}
		}()

		var treeDBs []string

		if treeDB != "" {
			if treeDBs, err = filepath.Glob(treeDB); err != nil {
				return fmt.Errorf("error determining paths of tree dbs: %w", err)
			}
		}

		n, err := treegen.BackupTree(env, collections, treeDBs...)
		if err != nil {
			return fmt.Errorf("error gathering backed up collection data: %w", err)
		}

		b := bufio.NewWriter(f)

		if err := tree.Serialise(b, n); err != nil {
			return fmt.Errorf("error writing tree file: %w", err)
		}

		if err := b.Flush(); err != nil {
			return fmt.Errorf("error flushing tree db: %w", err)
		}

		return nil
	},
}

func getCollectionTransformers(config *config.Config) (map[string]transformer.PathTransformer, error) {
	collections := make(map[string]transformer.PathTransformer)

	for collection, tx := range config.GetCollections() {
		fn, err := transformer.MakePathTransformer(tx)
		if err != nil {
			return nil, fmt.Errorf("error creating transformer: %w", err)
		}

		collections[collection] = fn
	}

	return collections, nil
}

func parseIRODSEnvFile(path string) (iron.Env, error) {
	env, _, err := cli.FileLoader(path)(context.Background(), "")

	return env, err
}

func init() {
	treeDBCmd := &cobra.Command{
		Use:   "treedb [local|remote]",
		Short: "Create tree databases for either local or remote data",
	}

	treeDBCmd.AddCommand(treeDBLocalCmd)
	treeDBCmd.AddCommand(treeDBRemoteCmd)

	treeDBRemoteCmd.Flags().StringVarP(&configPath, "config", "c", "", "backup config")
	treeDBRemoteCmd.Flags().StringVarP(&treeDB, "treedbs", "t", "", "glob to tree dbs")
	treeDBRemoteCmd.Flags().StringVarP(&ibackupConfig, "ibackup", "i",
		os.Getenv(cmd.ConfigKey), "ibackup config")
	treeDBRemoteCmd.Flags().StringVar(&irodsEnv, "irods",
		os.Getenv("IRODS_ENVIRONMENT_FILE"), "irods environment file")

	treeDBRemoteCmd.MarkFlagRequired("config") //nolint:errcheck

	RootCmd.AddCommand(treeDBCmd)
}
