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
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wtsi-hgi/backup-plans/config"
	"github.com/wtsi-hgi/backup-plans/db"
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
--listen server port to listen on

--config should be the location of a Yaml config file, which should have the
following structure:

The following is the config structure:

ibackup:
  servers:
    "serverName1":
      addr: ibackup01.server:1234
      cert: /path/to/cert/pem
      username: admin1
      token: /path/to/token
	"serverName1":
      addr: ibackup02.server:1234
      cert: /path/to/cert2/pem
      username: admin2
      token: /path/to/token2
  pathtoserver:
    ^/some/path/:
      servername: serverName1
      transformer: prefix=/some/path/:/some/remote/path/
    ^/some/o*/path/:
      servername: serverName2
      transformer: prefix=/some/:/remote/
IBackupCacheDuration: 3600
BOMFile: /path/to//bom.areas
OwnersFile: /path/to/owners
AdminGroup: 15770
ReloadTime: 3600
ReportingRoots:
 - /path/to/be/reported/
 - /other-path/to/be/reported/

The key of the Servers map is the server name, as used in the PathToServer
map.

The key of the PathToServer map is a regexp string that will be matched
against path; a matching path will use the server details associated with the
regexp.

The IBackupCacheDuration is a number of seconds until the ibackup set cache will
be updated.

OwnersFile and BOMFile strings are paths to CSV files with the following
formats:

Owners:

	GID,OwnerName

BOM:

	GroupName,BOMName


The AdminGroup is used to specify an admin group id to allow users of that group
visibility permissions within the DiskTree.
				
If the ReloadTime setting is non-zero, the config will be reloaded after
waiting that many seconds. Reloading the config will rebuild all structures,
while keeping any caches intact.

The ReportingRoots is a list of paths that will appear on the Top Level Report.
`,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		envMap := map[string]string{
			"BACKUP_PLANS_CONNECTION": "plan",
		}

		return checkEnvVarFlags(cmd, envMap)
	},
	RunE: func(_ *cobra.Command, args []string) error {
		config, err := config.ParseConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to process config file: %w", err)
		}

		d, err := db.Init(planDB)
		if err != nil {
			return err
		}

		return server.Start(fmt.Sprintf(":%d", serverPort), d, getUser, config, args...)
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
