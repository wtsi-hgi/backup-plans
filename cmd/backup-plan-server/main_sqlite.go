//go:build sqlite
// +build sqlite

package main

import _ "modernc.org/sqlite"

func init() {
	sqlDriver = "sqlite"
}
