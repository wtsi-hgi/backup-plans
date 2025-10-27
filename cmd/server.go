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

	sqlDriver = "mysql"
)

// serverCmd represents the server command.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web server",
	Long: `Start the web server.

`,
	RunE: func(_ *cobra.Command, args []string) error {
		d, err := db.Init(sqlDriver, os.Getenv("BACKUP_MYSQL_URL"))
		if err != nil {
			return err
		}

		return server.Start(fmt.Sprintf(":%d", serverPort), d, getUser, reportRoots, adminGroup, args...) //nolint:gosec
	},
}

func init() {
	RootCmd.AddCommand(serverCmd)

	// flags specific to this sub-command
	serverCmd.Flags().Uint16VarP(&serverPort, "port", "p", defaultPort,
		"port to start server on")
	serverCmd.Flags().Uint32VarP(&adminGroup, "admin", "a", 0, "admin groups that can see the entire tree")
	serverCmd.Flags().StringSliceVarP(&reportRoots, "report", "r", nil, "reporting root, can be supplied more than once")
}

func getUser(r *http.Request) string {
	return r.FormValue("user")
}
