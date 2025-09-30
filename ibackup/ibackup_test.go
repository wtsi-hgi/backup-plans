package ibackup

import (
	"os/user"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/ugorji/go/codec"
	"github.com/wtsi-hgi/backup-plans/backups"
	"github.com/wtsi-hgi/backup-plans/internal"
	"github.com/wtsi-hgi/backup-plans/rules"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"go.etcd.io/bbolt"
)

func TestIbackup(t *testing.T) {
	Convey("Given a new ibackup server", t, func() {
		s, addr, certPath, dfn, err := internal.NewTestIbackupServer(t)
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
			setNameWithPrefix := SetNamePrefix + setName

			So(Backup(client, setName, u.Username, []string{}, 0), ShouldBeNil)
			So(RunBackups(client), ShouldBeNil)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			So(Backup(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 0), ShouldBeNil)
			So(RunBackups(client), ShouldBeNil)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			before := time.Now()

			So(Backup(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1), ShouldBeNil)
			So(RunBackups(client), ShouldBeNil)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(checkTimes(sets), ShouldResemble, []*set.Set{
				{
					Name:        setNameWithPrefix,
					Requester:   u.Username,
					Transformer: "humgen",
					Description: "automatic backup set",
					Metadata:    map[string]string{},
					NumFiles:    1,
					Missing:     1,
					Status:      set.Complete,
				},
			})

			lastCompleted, err := GetSetLastCompleted(client, setNameWithPrefix, u.Username)
			So(err, ShouldBeNil)
			So(lastCompleted, ShouldHappenAfter, before)

			Convey("You cannot update a sets files more often than the frequency allows", func() {
				got, err := client.GetSetByName(u.Username, setNameWithPrefix)
				So(err, ShouldBeNil)

				ld := time.Now().Add(-time.Hour)
				got.LastDiscovery = ld

				So(setSet(s, got), ShouldBeNil)
				So(Backup(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1), ShouldBeNil)
				So(RunBackups(client), ShouldBeNil)

				got, err = client.GetSetByName(u.Username, setNameWithPrefix)
				So(err, ShouldBeNil)
				So(got.LastDiscovery, ShouldEqual, ld)

				ld = time.Now().Add(-24 * time.Hour)
				got.LastDiscovery = ld

				So(setSet(s, got), ShouldBeNil)

				So(Backup(client, setName, u.Username, []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1), ShouldBeNil)
				So(RunBackups(client), ShouldBeNil)

				got, err = client.GetSetByName(u.Username, setNameWithPrefix)
				So(err, ShouldBeNil)
				So(got.LastDiscovery, ShouldHappenAfter, ld)
			})

			Convey("You can get the last backup status of automatically created sets", func() {
				fs := func(yield func(backups.ProjectAction, []string) bool) {
					if !yield(backups.ProjectAction{
						Action:    rules.ActionBackup,
						Name:      setName,
						Requestor: u.Username,
						Frequency: 7,
					}, []string{}) {
						return
					}

					if !yield(backups.ProjectAction{
						Action:    rules.ActionBackup,
						Name:      setName,
						Requestor: u.Username,
						Frequency: 0,
					}, []string{}) {
						return
					}

					if !yield(backups.ProjectAction{
						Action:    rules.ActionManualBackup,
						Name:      setName,
						Requestor: u.Username,
						Metadata:  "manualSetName",
					}, []string{}) {
						return
					}

					if !yield(backups.ProjectAction{
						Action:    rules.ActionManualBackup,
						Name:      setName,
						Requestor: u.Username,
						Metadata:  setNameWithPrefix,
					}, []string{}) {

						return
					}
				}

				backupActivity, err := GetBackupActivity(client, fs)
				So(err, ShouldBeNil)
				So(backupActivity, ShouldNotBeNil)
				So(len(backupActivity), ShouldEqual, 4)

				So(backupActivity[0].Requester, ShouldEqual, u.Username)
				So(backupActivity[0].Name, ShouldEqual, setName)
				So(backupActivity[0].LastSuccess, ShouldHappenAfter, before)

				So(backupActivity[1].Requester, ShouldEqual, u.Username)
				So(backupActivity[1].Name, ShouldEqual, setName)
				So(backupActivity[1].LastSuccess, ShouldEqual, backupActivity[0].LastSuccess)

				So(backupActivity[2].Requester, ShouldEqual, u.Username)
				So(backupActivity[2].Name, ShouldEqual, setName)
				So(backupActivity[2].LastSuccess, ShouldEqual, time.Time{})

				So(backupActivity[3].Requester, ShouldEqual, u.Username)
				So(backupActivity[3].Name, ShouldEqual, setName)
				So(backupActivity[3].LastSuccess, ShouldEqual, backupActivity[0].LastSuccess)
			})
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
