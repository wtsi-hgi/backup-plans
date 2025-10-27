/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Sendu Bala <sb10@sanger.ac.uk>
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

package plandb

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey" //nolint:revive,staticcheck
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/testdb"
)

const defaultFrequency = 7

// CreateTestDatabase creates a new temporary database for testing and returns
// the db handle and the connection string. The database will be automatically
// closed at the end of the test. For sqlite3, the connection string is the path
// to the database file. For mysql, it is the full connection string including
// user, password, host, port and dbname.
func CreateTestDatabase(t *testing.T) (*db.DB, string) {
	t.Helper()

	driver, connection := testdb.GetTestDriverConnection(t)
	d, err := db.Init(driver, connection)
	So(err, ShouldBeNil)

	Reset(func() { d.Close() })

	return d, connection
}

// PopulateExamplePlanDB populates a plan database with some example data. It
// returns the db handle and the connection string. The database will be
// automatically closed at the end of the test. For sqlite3, the connection
// string is the path to the database file. For mysql, it is the full
// connection string including user, password, host, port and dbname.
func PopulateExamplePlanDB(t *testing.T) (*db.DB, string) { //nolint:funlen
	t.Helper()

	testDB, connectionStr := CreateTestDatabase(t)

	userA := "userA"
	userB := "userB"

	dirA := &db.Directory{
		Path:      "/lustre/scratch123/humgen/a/b/",
		ClaimedBy: userA,
	}
	dirB := &db.Directory{
		Path:      "/lustre/scratch123/humgen/a/c/",
		ClaimedBy: userB,
	}

	So(testDB.CreateDirectory(dirA), ShouldBeNil)
	So(testDB.CreateDirectory(dirB), ShouldBeNil)

	reviewDate := time.Now().Add(24 * time.Hour).UTC().Truncate(1 * time.Second).Unix() //nolint:mnd
	removeDate := time.Now().Add(48 * time.Hour).UTC().Truncate(1 * time.Second).Unix() //nolint:mnd

	ruleA := &db.Rule{
		BackupType: db.BackupIBackup,
		Match:      "*.jpg",
		Frequency:  defaultFrequency,
		ReviewDate: reviewDate,
		RemoveDate: removeDate,
	}
	ruleB := &db.Rule{
		BackupType: db.BackupNone,
		Match:      "temp.jpg",
		Frequency:  defaultFrequency,
		ReviewDate: reviewDate,
		RemoveDate: removeDate,
	}
	ruleC := &db.Rule{
		BackupType: db.BackupManual,
		Match:      "*.txt",
		Metadata:   "manualSetName",
		ReviewDate: reviewDate,
		RemoveDate: removeDate,
	}

	So(testDB.CreateDirectoryRule(dirA, ruleA), ShouldBeNil)
	So(testDB.CreateDirectoryRule(dirA, ruleB), ShouldBeNil)
	So(testDB.CreateDirectoryRule(dirB, ruleC), ShouldBeNil)

	return testDB, connectionStr
}
