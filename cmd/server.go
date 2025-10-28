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
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/server"
)

const defaultPort = 8080

// options for this cmd.
var (
	serverPort  uint16
	adminGroup  uint32
	reportRoots []string
)

// serverCmd represents the server command.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web server",
	Long: `Start the web server.

--plan should be a connection string for the plan database.

For sqlite, say:
  sqlite3:/path/to/plan.db

For mysql, say:
  mysql:user:password@tcp(host:port)/dbname

It is recommended to use the environment variable "BACKUP_MYSQL_URL" for this
to maintain password security.

--tree should be generated using the db command.
--admin specify admin group id to allow users of that group visibility permission
--report can be supplied multiple times, specifies root to be reported on.
--port server port
`,
	RunE: func(_ *cobra.Command, args []string) error {
		d, err := db.Init(planDB)
		if err != nil {
			return err
		}

		return server.Start(fmt.Sprintf(":%d", serverPort), d, getUser, reportRoots, adminGroup, args...)
	},
}

func init() {
	RootCmd.AddCommand(serverCmd)

	// flags specific to this sub-command
	serverCmd.Flags().Uint16VarP(&serverPort, "port", "p", defaultPort,
		"port to start server on")
	serverCmd.Flags().Uint32VarP(&adminGroup, "admin", "a", 0, "admin group that can see the entire tree")
	serverCmd.Flags().StringSliceVarP(&reportRoots, "report", "r", nil, "reporting root, can be supplied more than once")
	backupCmd.Flags().StringVarP(&planDB, "plan", "p", os.Getenv("BACKUP_MYSQL_URL"),
		"sql connection string for your plan database")
}

func getUser(r *http.Request) string {
	return r.FormValue("user")
}
