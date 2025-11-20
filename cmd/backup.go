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
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/backups"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"golang.org/x/sys/unix"
	"vimagination.zapto.org/tree"
)

// options for this cmd.
var planDB string
var treeDB string
var configPath string

// serverCmd represents the server command.
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Use iBackup to backup files in a plan db.",
	Long: `Use iBackup to backup files in a plan db.

--plan should be a connection string for the plan database.

For sqlite, say:
  sqlite3:/path/to/plan.db

For mysql, say:
  mysql:user:password@tcp(host:port)/dbname

It is recommended to use the environment variable "BACKUP_PLANS_CONNECTION" for this
to maintain password security.

--tree should be generated using the db command.

--ibackup ibackup server url
	env: IBACKUP_SERVER_URL

--cert ibackup server authentication certificate
	env: IBACKUP_SERVER_CERT
`,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		envMap := map[string]string{
			"BACKUP_PLANS_CONNECTION": "plan",
		}

		return checkEnvVarFlags(cmd, envMap)
	},
	RunE: func(_ *cobra.Command, _ []string) error {
		config, err := parseConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}

		client, err := ibackup.New(config.IBackup)
		if err != nil {
			if !ibackup.IsOnlyConnectionErrors(err) {
				return err
			}

			slog.Warn("ibackup connection errors", "errs", err)
		}

		planDB, err := db.Init(planDB)
		if err != nil {
			return fmt.Errorf("failed to open db: %w", err)
		}
		defer planDB.Close()

		treeNode, dfn, err := openTree(treeDB)
		if err != nil {
			return fmt.Errorf("\n failed to open tree db: %w", err)
		}
		defer dfn()

		setInfos, err := backups.Backup(planDB, treeNode, client)
		if err != nil {
			err = fmt.Errorf("\n failed to back up files: %w", err)
		}

		for _, setIn := range setInfos {
			cliPrintf("ibackup set '%s' created for %s with %v files\n", setIn.BackupSetName, setIn.Requestor, setIn.FileCount)
		}

		return err
	},
}

func init() {
	RootCmd.AddCommand(backupCmd)

	// flags specific to this sub-command
	backupCmd.Flags().StringVarP(&planDB, "plan", "p", os.Getenv("BACKUP_PLANS_CONNECTION"),
		"sql connection string for your plan database")
	backupCmd.Flags().StringVarP(&treeDB, "tree", "t", "",
		"Path to tree db file, usually generated using db cmd")
	backupCmd.Flags().StringVarP(&configPath, "config", "c", "", "ibackup config")

	backupCmd.MarkFlagRequired("tree")   //nolint:errcheck
	backupCmd.MarkFlagRequired("config") //nolint:errcheck
}

func checkEnvVarFlags(cmd *cobra.Command, envMap map[string]string) error {
	for env := range envMap {
		if v, err := cmd.Flags().GetString(envMap[env]); err != nil {
			return fmt.Errorf("failed to get flag %s: %w", envMap[env], err)
		} else if v == "" {
			return fmt.Errorf("--%s must be set when env variable '%s' is not", envMap[env], env) //nolint:err113
		}
	}

	return nil
}

func openTree(path string) (*tree.MemTree, func(), error) {
	f, size, err := openAndSize(path)
	if err != nil {
		return nil, nil, err
	}

	data, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		f.Close()

		return nil, nil, err
	}

	fn := func() {
		unix.Munmap(data) //nolint:errcheck
		f.Close()
	}

	db, err := tree.OpenMem(data)
	if err != nil {
		fn()

		return nil, nil, fmt.Errorf("error opening tree: %w", err)
	}

	return db, fn, nil
}

func openAndSize(path string) (*os.File, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()

		return nil, 0, err
	}

	return f, int(stat.Size()), nil
}
