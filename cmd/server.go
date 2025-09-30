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
	"github.com/wtsi-hgi/backup-plans/frontend"
	"github.com/wtsi-hgi/backup-plans/server"
)

// options for this cmd.
var serverPort int

// serverCmd represents the server command.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web server",
	Long: `Start the web server.

`,
	RunE: func(_ *cobra.Command, _ []string) error {
		d, err := db.Init("mysql", os.Getenv("BACKUP_MYSQL_URL")+"?parseTime=true")
		if err != nil {
			return err
		}

		s, err := server.New(d, getUser)
		if err != nil {
			return err
		}

		for _, db := range os.Args[1:] {
			fmt.Println("Loading ", db)

			if err := s.AddTree(db); err != nil {
				return err
			}
		}

		fmt.Println("Serving...")

		http.Handle("/api/whoami", http.HandlerFunc(s.WhoAmI))
		http.Handle("/api/tree", http.HandlerFunc(s.Tree))
		http.Handle("/api/dir/claim", http.HandlerFunc(s.ClaimDir))
		http.Handle("/api/dir/pass", http.HandlerFunc(s.PassDirClaim))
		http.Handle("/api/dir/revoke", http.HandlerFunc(s.RevokeDirClaim))
		http.Handle("/api/rules/create", http.HandlerFunc(s.CreateRule))
		http.Handle("/api/rules/get", http.HandlerFunc(s.GetRules))
		http.Handle("/api/rules/update", http.HandlerFunc(s.UpdateRule))
		http.Handle("/api/rules/remove", http.HandlerFunc(s.RemoveRule))
		http.Handle("/", http.HandlerFunc(frontend.Serve))

		return http.ListenAndServe(fmt.Sprintf(":%d", serverPort), nil)

	},
}

func init() {
	RootCmd.AddCommand(serverCmd)

	// flags specific to this sub-command
	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 12345,
		"port to start server on")
}

func getUser(r *http.Request) string {
	return r.FormValue("user")
}
