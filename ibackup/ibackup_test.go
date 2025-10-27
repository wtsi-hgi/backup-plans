package ibackup

import (
	"os/user"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/ugorji/go/codec"
	"github.com/wtsi-hgi/backup-plans/internal/ibackup"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"go.etcd.io/bbolt"
)

func TestIbackup(t *testing.T) {
	Convey("Given a new ibackup server", t, func() {
		s, addr, certPath, dfn, err := ibackup.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		u, err := user.Current()
		So(err, ShouldBeNil)

		client, err := Connect(addr, certPath)
		So(err, ShouldBeNil)

		Convey("You can create backup sets", func() {
			sets, err := client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			setName := "mySet"

			runTestBackups(client, setName, u.Username, []string{}, 0)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			runTestBackups(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 0)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			before := time.Now()

			runTestBackups(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(checkTimes(sets), ShouldResemble, []*set.Set{
				{
					Name:        setName,
					Requester:   u.Username,
					Transformer: "humgen",
					Metadata:    map[string]string{},
					NumFiles:    1,
					Missing:     1,
					Status:      set.Complete,
				},
			})

			sba, err := GetBackupActivity(client, setName, u.Username)
			So(err, ShouldBeNil)
			So(sba.LastSuccess, ShouldHappenAfter, before)

			Convey("You cannot update a sets files more often than the frequency allows", func() {
				got, err := client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)

				ld := time.Now().Add(-time.Hour)
				got.LastDiscovery = ld

				So(setSet(s, got), ShouldBeNil)
				runTestBackups(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1)

				got, err = client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)
				So(got.LastDiscovery, ShouldEqual, ld)

				ld = time.Now().Add(-24 * time.Hour)
				got.LastDiscovery = ld

				So(setSet(s, got), ShouldBeNil)

				runTestBackups(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1)

				got, err = client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)
				So(got.LastDiscovery, ShouldHappenAfter, ld)
			})

			Convey("You can get the last backup status of automatically created sets", func() {
				backupActivity, err := GetBackupActivity(client, setName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity, ShouldNotBeNil)
				So(backupActivity.Requester, ShouldEqual, u.Username)
				So(backupActivity.Name, ShouldEqual, setName)
				So(backupActivity.LastSuccess, ShouldHappenAfter, before)

				_, err = GetBackupActivity(client, "invalidSetName", u.Username)
				So(err, ShouldNotBeNil)

				manualSetName := "manualSetName"
				files := []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}
				manualSet := &set.Set{
					Name:        manualSetName,
					Requester:   u.Username,
					Transformer: getTransformer(files[0]),
					Description: "manual backup set",
				}

				err = client.AddOrUpdateSet(manualSet)
				So(err, ShouldBeNil)

				backupActivity, err = GetBackupActivity(client, manualSetName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity, ShouldNotBeNil)
				So(backupActivity.Requester, ShouldEqual, u.Username)
				So(backupActivity.Name, ShouldEqual, manualSetName)
				So(backupActivity.LastSuccess, ShouldEqual, time.Time{})
			})

			// TODO: try calling GetBackupActivity() for each dir in the
			// plan database, plus each rule that is for manual backup Functions
			// somewhere that do the above. Maybe these are tests in the db pkg?
		})
	})
}

func checkTimes(sets []*set.Set) []*set.Set {
	for _, set := range sets {
		So(set.StartedDiscovery, ShouldHappenWithin, time.Minute, time.Now())
		So(set.LastDiscovery, ShouldHappenWithin, time.Minute, time.Now())
		So(set.LastCompleted, ShouldHappenWithin, time.Minute, time.Now())

		set.StartedDiscovery = time.Time{}
		set.LastDiscovery = time.Time{}
		set.LastCompleted = time.Time{}
	}

	return sets
}

type boltDB struct {
	db *bbolt.DB
	ch codec.Handle
}

func getDB(s *server.Server) *boltDB {
	return (*struct {
		gas.Server
		db *boltDB
	})(unsafe.Pointer(s)).db
}

func setSet(s *server.Server, got *set.Set) error {
	db := getDB(s)

	return db.db.Update(func(tx *bbolt.Tx) error {
		var encoded []byte
		enc := codec.NewEncoderBytes(&encoded, db.ch)
		enc.MustEncode(got)

		b := tx.Bucket([]byte("sets"))

		return b.Put([]byte(got.ID()), encoded)
	})
}

func runTestBackups(client *server.Client, setname, requester string, files []string, frequency int) {
	err := Backup(client, setname, requester, files, frequency)
	So(err, ShouldBeNil)
}
