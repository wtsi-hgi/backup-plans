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

package testdb

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey" //nolint:revive,staticcheck
	"github.com/wtsi-hgi/backup-plans/db"
	_ "modernc.org/sqlite" //
)

func CreateTestDatabase(t *testing.T) *db.DB { //nolint:funlen
	t.Helper()

	oldTmp := os.Getenv("TMPDIR")

	if _, err := os.Stat("/dev/shm"); err == nil {
		os.Setenv("TMPDIR", "/dev/shm")
	}

	os.Setenv("TMPDIR", oldTmp)

	d, err := db.Init("sqlite", filepath.Join(t.TempDir(), "db?journal_mode=WAL&_pragma=foreign_keys(1)"))
	So(err, ShouldBeNil)

	Reset(func() { d.Close() })

	return d
}
