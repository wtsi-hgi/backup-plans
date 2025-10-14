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

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/backups"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
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

//TODO: give an example planDB connection string here
--plan should be a connection string for the plan database. 
Optional prefixes are:
  plan: for sqlite3 (default)
  file: for mysql

--tree should be generated using the db command.
`,

	RunE: func(_ *cobra.Command, args []string) error {
		client, err := ibackup.Connect(ibackupURL, cert)
		if err != nil {
			return fmt.Errorf("failed to connect to ibackup server: %w", err)
		}

		// TODO: detect driver from planDB string, see code we wrote previously
		// in old repo
		// old repo asks for driver in args (backup-plan-ui/main.go line 34)

		// 1:

		// // default driver is sqlite3
		// driver := "sqlite3"
		// pathItems := strings.Split(planDB, ":")
		// if len(pathItems) > 1 {
		// 	switch pathItems[0] {
		// 	case "plan":
		// 		driver = "sqlite3"
		// 	case "file":
		// 		driver = "mysql"
		// 	default:
		// 		return fmt.Errorf("unrecognised db driver: %s", pathItems[0])
		// 	}
		// }

		// 2:

		// ext := path.Ext(planDB)
		// driver := "sqlite3"
		// switch ext {
		// case ".db", ".sqlite", ".sqlite3":
		// 	driver = "sqlite3"
		// case ".mysql":
		// 	driver = "mysql"
		// default:
		// 	return fmt.Errorf("unrecognised db driver from file extension: %s", ext)
		// }

		driver := "sqlite3"
		planDB, err := db.Init(driver, planDB)
		if err != nil {
			return fmt.Errorf("failed to open db: %w", err)
		}
		defer planDB.Close() //nolint:errcheck

		// sort treeNode incorrect type
		treeNode, err := tree.OpenFile(treeDB)
		if err != nil {
			return fmt.Errorf("failed to open tree db: %w", err)
		}
		defer treeNode.Close() //nolint:errcheck

		setInfos, err := backups.Backup(planDB, treeNode, client)
		if err != nil {
			return fmt.Errorf("failed to back up files: %w", err)
		}

		for _, setIn := range setInfos {
			fmt.Printf("ibackup set '%s' created for %s with %v files\n", setIn.BackupSetName, setIn.Requestor, setIn.FileCount)
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

	backupCmd.MarkFlagRequired("plan")
	backupCmd.MarkFlagRequired("tree")
	backupCmd.MarkFlagRequired("ibackup")
	backupCmd.MarkFlagRequired("cert")
}
