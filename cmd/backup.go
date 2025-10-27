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
	"strings"

	_ "github.com/go-sql-driver/mysql" //
	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/backups"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	_ "modernc.org/sqlite" //
	"vimagination.zapto.org/tree"
)

// options for this cmd.
var ibackupURL string
var planDB string
var treeDB string
var cert string

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

--tree should be generated using the db command.
`,

	RunE: func(_ *cobra.Command, args []string) error { //nolint:revive
		client, err := ibackup.Connect(ibackupURL, cert)
		if err != nil {
			return fmt.Errorf("failed to connect to ibackup server: %w", err)
		}

		driver := "sqlite"

		pathItems := strings.SplitN(planDB, ":", 2) //nolint:mnd
		strings.Cut(planDB, ":")
		if len(pathItems) > 1 {
			switch pathItems[0] {
			case "sqlite", "sqlite3":
			case "mysql":
				driver = "mysql"
			default:
				return fmt.Errorf("unrecognised db driver: %s", pathItems[0]) //nolint:err113
			}
		}

		path := pathItems[len(pathItems)-1]
		planDB, err := db.Init(driver, path)
		if err != nil {
			return fmt.Errorf("failed to open db: %w", err)
		}
		defer planDB.Close()

		treeNode, err := tree.OpenFile(treeDB)
		if err != nil {
			return fmt.Errorf("\n failed to open tree db: %w", err)
		}
		defer treeNode.Close()

		setInfos, err := backups.Backup(planDB, treeNode, client)
		if err != nil {
			return fmt.Errorf("\n failed to back up files: %w", err)
		}

		for _, setIn := range setInfos {
			cliPrintf("ibackup set '%s' created for %s with %v files\n", setIn.BackupSetName, setIn.Requestor, setIn.FileCount)
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(backupCmd)

	// flags specific to this sub-command
	backupCmd.Flags().StringVarP(&planDB, "plan", "p", "",
		"sql connection string for your plan database")
	backupCmd.Flags().StringVarP(&treeDB, "tree", "t", "",
		"Path to tree db file, usually generated using db cmd")
	backupCmd.Flags().StringVarP(&ibackupURL, "ibackup", "i", "",
		"ibackup server url")
	backupCmd.Flags().StringVarP(&cert, "cert", "c", "", "Path to ibackup server certificate file")

	backupCmd.MarkFlagRequired("plan")    //nolint:errcheck
	backupCmd.MarkFlagRequired("tree")    //nolint:errcheck
	backupCmd.MarkFlagRequired("ibackup") //nolint:errcheck
	backupCmd.MarkFlagRequired("cert")    //nolint:errcheck
}
