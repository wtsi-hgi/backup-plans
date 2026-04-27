/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
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
	"os"

	"github.com/kuleuven/iron"
	"github.com/kuleuven/iron/cmd/iron/cli"
	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/config"
	"github.com/wtsi-hgi/backup-plans/treegen"
	"github.com/wtsi-hgi/ibackup/cmd"
	"github.com/wtsi-hgi/ibackup/transformer"
	"vimagination.zapto.org/tree"
)

var (
	ibackupConfig, irodsEnv string

	ErrMissingOutput = errors.New("missing backuptree db output file")
)

// backupdbCmd represents the backupdb command.
var backupdbCmd = &cobra.Command{
	Use:   "backupdb <backuptree.db>",
	Short: "Create tree database from files backed-up in iRODS via iBackup.",
	Long: `Create tree database from files backed-up in iRODS via iBackup FOFN server.

Provide the path to the output location for the tree of backed-up
files/collections.

--config should be the location of a Yaml config file, which should have the
following structure:

Collections:
  /collection/1/: transformer_1
  /collection/2/: transformer_2

With the transformers corresponding either to prefix transformers or to those
specified in the ibackup config file, the location of which should be specified
with either the --ibackup flag or the IBACKUP_CONFIG env var.

In addition, the irods environmental file should be specified either wit the
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

		n, err := treegen.BackupTree(env, collections)
		if err != nil {
			return fmt.Errorf("error gathering backed-up collection data: %w", err)
		}

		b := bufio.NewWriter(f)

		if err := tree.Serialise(b, n); err != nil {
			return fmt.Errorf("error writing tree file: %w", err)
		}

		if err := b.Flush(); err != nil {
			return fmt.Errorf("error flushing tree db: %w", err)
		}

		return f.Close()
	},
}

func init() {
	RootCmd.AddCommand(backupdbCmd)

	backupdbCmd.Flags().StringVarP(&configPath, "config", "c", "", "backup config")
	backupdbCmd.Flags().StringVarP(&ibackupConfig, "ibackup", "i",
		os.Getenv(cmd.ConfigKey), "ibackup config")
	backupdbCmd.Flags().StringVar(&irodsEnv, "irods",
		os.Getenv("IRODS_ENVIRONMENT_FILE"), "irods environment file")

	backupdbCmd.MarkFlagRequired("config") //nolint:errcheck
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
