/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
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

package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unsafe"

	_ "github.com/go-sql-driver/mysql"
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/server"
)

var sqlDriver = "mysql"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

type reportRoots []string

func (r reportRoots) String() string {
	return fmt.Sprint([]string(r))
}

func (r *reportRoots) Set(val string) error {
	*r = append(*r, val)

	return nil
}

func run() error {
	var (
		port   uint64
		report reportRoots
	)

	flag.Uint64Var(&port, "port", 12345, "port to start server on")
	flag.Var(&report, "report", "reporting root, can be supplied more than once")

	flag.Parse()

	d, err := db.Init(sqlDriver, os.Getenv("BACKUP_MYSQL_URL"))
	if err != nil {
		return err
	}

	return server.Start(fmt.Sprintf(":%d", port), d, getUser, report, flag.Args()...)

}

func getUser(r *http.Request) string {
	for _, cookie := range r.Cookies() {
		if cookie.Name == "nginxauth" {
			data, err := base64.StdEncoding.DecodeString(cookie.Value)
			if err != nil {
				return ""
			}

			return strings.SplitN(unsafe.String(unsafe.SliceData(data), len(data)), ":", 2)[0]
		}
	}

	return ""
}
