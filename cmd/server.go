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
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	"github.com/wtsi-hgi/backup-plans/server"
)

const defaultPort = 8080

// options for this cmd.
var (
	serverPort  uint16
	adminGroup  uint32
	reportRoots []string
	owners      string
	bom         string
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

It is recommended to use the environment variable "BACKUP_PLANS_CONNECTION" for this
to maintain password security.

--tree should be generated using the db command.
--admin specify admin group id to allow users of that group visibility permission
--report can be supplied multiple times, specifies root to be reported on.
--listen server port to listen on

--ibackup ibackup server url
	env: IBACKUP_SERVER_URL

--cert ibackup server authentication certificate
	env: IBACKUP_SERVER_CERT

--owners path to Owners CSV file containing two columns: GID,Owner
--bom path to BOM CSV file containing two columns: GroupName,BOM
`,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		envMap := map[string]string{
			"BACKUP_PLANS_CONNECTION": "plan",
		}

		return checkEnvVarFlags(cmd, envMap)
	},
	RunE: func(_ *cobra.Command, args []string) error {
		config, err := parseConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}

		d, err := db.Init(planDB)
		if err != nil {
			return err
		}

		client, err := ibackup.New(config.IBackup)
		if err != nil {
			if !ibackup.IsOnlyConnectionErrors(err) {
				return err
			}

			slog.Warn("ibackup connection errors", "errs", err)
		}

		return server.Start(fmt.Sprintf(":%d", serverPort), d, getUser, reportRoots, adminGroup, client, owners, bom, args...)
	},
}

func init() {
	RootCmd.AddCommand(serverCmd)

	// flags specific to this sub-command
	serverCmd.Flags().Uint16VarP(&serverPort, "listen", "l", defaultPort,
		"port to start server on")
	serverCmd.Flags().Uint32VarP(&adminGroup, "admin", "a", 0, "admin group that can see the entire tree")
	serverCmd.Flags().StringSliceVarP(&reportRoots, "report", "r", nil, "reporting root, can be supplied more than once")
	serverCmd.Flags().StringVarP(&planDB, "plan", "p", os.Getenv("BACKUP_PLANS_CONNECTION"),
		"sql connection string for your plan database")
	serverCmd.Flags().StringVarP(&configPath, "config", "c", "", "ibackup config")
	serverCmd.Flags().StringVarP(&owners, "owners", "o", owners, "path to owners CSV file")
	serverCmd.Flags().StringVarP(&bom, "bom", "b", bom, "path to bom area CSV file")

	serverCmd.MarkFlagRequired("tree")   //nolint:errcheck
	serverCmd.MarkFlagRequired("config") //nolint:errcheck
}

func getUser(r *http.Request) string {
	for _, cookie := range r.Cookies() {
		if cookie.Name == "nginxauth" {
			data, err := base64.StdEncoding.DecodeString(cookie.Value)
			if err != nil {
				return ""
			}

			user, _, _ := strings.Cut(string(data), ":")

			return user
		}
	}

	return ""
}
