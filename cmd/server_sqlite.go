//go:build sqlite
// +build sqlite

package cmd

import _ "modernc.org/sqlite"

func init() {
	sqlDriver = "sqlite"
}
