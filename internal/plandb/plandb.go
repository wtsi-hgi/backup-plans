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
	"slices"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey" //nolint:revive,staticcheck
	"github.com/wtsi-hgi/backup-plans/db"
	"github.com/wtsi-hgi/backup-plans/internal/directories"
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

	connection := testdb.GetTestDriverConnection(t)
	d, err := db.Init(connection)
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

func ExampleTree() *directories.Root { //nolint:ireturn,nolintlint
	dirRoot := directories.NewRoot("/lustre/", 12345)                //nolint:mnd
	humgen := dirRoot.SetMeta(99, 98, 1).AddDirectory("scratch123"). //nolint:mnd
										SetMeta(1, 1, 98765).AddDirectory("humgen").SetMeta(1, 1, 98765) //nolint:mnd

	humgen.AddDirectory("a").SetMeta(99, 98, 1).AddDirectory("b").SetMeta(1, 1, 98765). //nolint:mnd
												AddDirectory("testdir").SetMeta(2, 1, 12349).     //nolint:mnd
												AddDirectory("testdirchild").SetMeta(2, 1, 12349) //nolint:mnd
	directories.AddFile(&dirRoot.Directory, "scratch123/humgen/a/b/1.jpg", 1, 1, 9, 98766)            //nolint:mnd
	directories.AddFile(&dirRoot.Directory, "scratch123/humgen/a/b/2.jpg", 1, 2, 8, 98767)            //nolint:mnd
	directories.AddFile(&dirRoot.Directory, "scratch123/humgen/a/b/3.txt", 1, 2, 8, 98767)            //nolint:mnd
	directories.AddFile(&dirRoot.Directory, "scratch123/humgen/a/b/temp.jpg", 1, 2, 8, 98767)         //nolint:mnd
	directories.AddFile(&dirRoot.Directory, "scratch123/humgen/a/b/testdir/test.txt", 2, 1, 6, 12346) //nolint:mnd

	humgen.AddDirectory("a").AddDirectory("c").SetMeta(2, 1, 12349)                        //nolint:mnd
	directories.AddFile(&dirRoot.Directory, "scratch123/humgen/a/c/4.txt", 2, 1, 6, 12346) //nolint:mnd

	return dirRoot
}

func PopulateBigExamplePlanDB(t *testing.T) (*db.DB, string) { //nolint:funlen
	t.Helper()

	plandb, connectionStr := PopulateExamplePlanDB(t)

	reviewDate := time.Now().Add(24 * time.Hour).UTC().Truncate(1 * time.Second).Unix() //nolint:mnd
	removeDate := time.Now().Add(48 * time.Hour).UTC().Truncate(1 * time.Second).Unix() //nolint:mnd

	testdataA := []*db.Rule{
		{
			BackupType: db.BackupIBackup,
			Match:      "*.cram",
			Frequency:  defaultFrequency,
			ReviewDate: reviewDate,
			RemoveDate: removeDate,
		},
		{
			BackupType: db.BackupNone,
			Match:      "*.txt",
			Frequency:  defaultFrequency,
			ReviewDate: reviewDate,
			RemoveDate: removeDate,
		},
	}
	testdataB := []*db.Rule{
		{
			BackupType: db.BackupIBackup,
			Match:      "*.cram",
			Frequency:  defaultFrequency,
			ReviewDate: reviewDate,
			RemoveDate: removeDate,
		},
	}
	testdataC := []*db.Rule{
		{
			BackupType: db.BackupIBackup,
			Match:      "*.cram",
			Frequency:  defaultFrequency,
			ReviewDate: reviewDate,
			RemoveDate: removeDate,
		},
	}

	dirs := slices.Collect(plandb.ReadDirectories().Iter)
	dirA := &db.Directory{
		Path:      "/lustre/scratch123/humgen/a/b/newdir/",
		ClaimedBy: "userC",
	}
	dirB := dirs[1]
	dirC := &db.Directory{
		Path:      "/lustre/scratch123/humgen/a/b/newdir/testextradir/",
		ClaimedBy: "userD",
	}

	So(plandb.CreateDirectory(dirA), ShouldBeNil)
	// plandb.CreateDirectory(dirB)
	So(plandb.CreateDirectory(dirC), ShouldBeNil)

	for _, rule := range testdataA {
		So(plandb.CreateDirectoryRule(dirA, rule), ShouldBeNil)
	}

	for _, rule := range testdataB {
		So(plandb.CreateDirectoryRule(dirB, rule), ShouldBeNil)
	}

	for _, rule := range testdataC {
		So(plandb.CreateDirectoryRule(dirC, rule), ShouldBeNil)
	}

	return plandb, connectionStr
}

func ExampleTreeBig() *directories.Root {
	tree := ExampleTree()

	directories.AddFile(&tree.Directory, "scratch123/humgen/a/b/1.cram", 1, 1, 9, 98766)                       //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/b/newdir/testcram.cram", 1, 1, 9, 98766)         //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/b/newdir/test.txt", 1, 1, 9, 98766)              //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/c/2.cram", 1, 1, 9, 98766)                       //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/c/newdir/2.cram", 1, 1, 9, 98766)                //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/c/newdir/tmp.txt", 1, 1, 9, 98766)               //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/d/tmp.txt", 1, 1, 9, 98766)                      //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/b/newdir/testextradir/test.txt", 2, 1, 6, 12346) //nolint:mnd
	directories.AddFile(&tree.Directory, "scratch123/humgen/a/c/newdir/testextradir/test.txt", 2, 1, 6, 12346) //nolint:mnd

	return tree
}
