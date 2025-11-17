package ibackup_test

import (
	"os/user"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/ugorji/go/codec"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ib "github.com/wtsi-hgi/backup-plans/internal/ibackup"
	"github.com/wtsi-hgi/backup-plans/internal/testirods"
	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/ibackup/transfer"
	"go.etcd.io/bbolt"
)

var (
	twoyears         = time.Now().Add(time.Hour * 24 * 365 * 2) //nolint:gochecknoglobals
	twoYearsAddMonth = twoyears.Add(time.Hour * 24 * 30)        //nolint:gochecknoglobals
)

func TestIbackup(t *testing.T) {
	Convey("Given a new ibackup server", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		s, addr, certPath, dfn, err := ib.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		u, err := user.Current()
		So(err, ShouldBeNil)

		client, err := ibackup.Connect(addr, certPath)
		So(err, ShouldBeNil)

		mockClient := newMockClient(client)

		Convey("You can create backup sets", func() {
			sets, err := client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			setName := "mySet"

			err = ibackup.Backup(client, setName, u.Username, []string{}, 0, 0, 0)
			So(err, ShouldBeNil)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			before := time.Now()

			review := twoyears.Unix()
			remove := twoYearsAddMonth.Unix()
			setName += "2"

			runTestBackups(client, setName, u.Username,
				[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, review, remove)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(checkTimes(sets), ShouldResemble, []*set.Set{
				{
					Name:        setName,
					Requester:   u.Username,
					Transformer: "humgen",
					Metadata: map[string]string{
						transfer.MetaKeyReview:  ibackup.TimeToMeta(review),
						transfer.MetaKeyRemoval: ibackup.TimeToMeta(remove),
					},
					NumFiles: 1,
					Missing:  1,
					Status:   set.Complete,
				},
			})
			So(sets[0].Metadata[transfer.MetaKeyReview], ShouldNotBeBlank)
			So(sets[0].Metadata[transfer.MetaKeyRemoval], ShouldNotBeBlank)

			sba, err := ibackup.GetBackupActivity(client, setName, u.Username)
			So(err, ShouldBeNil)
			So(sba.LastSuccess, ShouldHappenAfter, before)

			Convey("You can create a freeze, which cannot be overwritten", func() {
				u.Username += "2"

				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 0, 0, 0)

				sba, err := ibackup.GetBackupActivity(client, setName, u.Username)
				So(err, ShouldBeNil)
				So(sba.LastSuccess, ShouldNotBeNil)

				err = ibackup.Backup(mockClient, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 0, 0, 0)

				So(err, ShouldEqual, ibackup.ErrFrozenSet)

				So(len(mockClient.Discoveries), ShouldBeZeroValue)
			})

			Convey("You cannot update a sets files more often than the frequency allows", func() {
				got, err := client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)

				ld := time.Now().Add(-time.Hour)
				got.LastDiscovery = ld

				So(setSet(s, got), ShouldBeNil)
				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, 0, 0)

				got, err = client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)
				So(got.LastDiscovery, ShouldEqual, ld)

				ld = time.Now().Add(-24 * time.Hour)
				got.LastDiscovery = ld

				So(setSet(s, got), ShouldBeNil)

				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, review, remove)

				got, err = client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)
				So(got.LastDiscovery, ShouldHappenAfter, ld)
			})

			Convey("You can get the last backup status of automatically created sets", func() {
				backupActivity, err := ibackup.GetBackupActivity(client, setName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity, ShouldNotBeNil)
				So(backupActivity.Requester, ShouldEqual, u.Username)
				So(backupActivity.Name, ShouldEqual, setName)
				So(backupActivity.LastSuccess, ShouldHappenAfter, before)

				_, err = ibackup.GetBackupActivity(client, "invalidSetName", u.Username)
				So(err, ShouldNotBeNil)

				manualSetName := "manualSetName"
				files := []string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}
				manualSet := &set.Set{
					Name:        manualSetName,
					Requester:   u.Username,
					Transformer: ibackup.GetTransformer(files[0]),
					Description: "manual backup set",
				}

				err = client.AddOrUpdateSet(manualSet)
				So(err, ShouldBeNil)

				backupActivity, err = ibackup.GetBackupActivity(client, manualSetName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity, ShouldNotBeNil)
				So(backupActivity.Requester, ShouldEqual, u.Username)
				So(backupActivity.Name, ShouldEqual, manualSetName)
				So(backupActivity.LastSuccess, ShouldEqual, time.Time{})
			})

			Convey("You can retrieve the last backup status from the cache", func() {
				cacheTimeout := time.Second * 2
				cache := ibackup.NewCache(client, cacheTimeout)
				backupActivity, err := cache.GetBackupActivity(setName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity, ShouldNotBeNil)
				So(backupActivity.Requester, ShouldEqual, u.Username)
				So(backupActivity.Name, ShouldEqual, setName)
				So(backupActivity.LastSuccess, ShouldHappenAfter, before)

				*(*string)(unsafe.Pointer(client)) = ""

				backupActivity, err = cache.GetBackupActivity(setName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity, ShouldNotBeNil)
				So(backupActivity.Requester, ShouldEqual, u.Username)
				So(backupActivity.Name, ShouldEqual, setName)
				So(backupActivity.LastSuccess, ShouldHappenAfter, before)
			})

			Convey("The cache updates after a specified amount of time", func() {
				cacheTimeout := time.Second * 1
				cache := ibackup.NewCache(client, cacheTimeout)
				backupActivity, err := cache.GetBackupActivity(setName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity.LastSuccess, ShouldHappenAfter, before)

				err = client.AddOrUpdateSet(sets[0])
				So(err, ShouldBeNil)

				backupActivity, err = cache.GetBackupActivity(setName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity.LastSuccess, ShouldHappenAfter, before)

				before = backupActivity.LastSuccess

				err = client.TriggerDiscovery(sets[0].ID(), false)
				So(err, ShouldBeNil)

				time.Sleep(2 * cacheTimeout)

				backupActivity, err = cache.GetBackupActivity(setName, u.Username)
				So(err, ShouldBeNil)
				So(backupActivity.LastSuccess, ShouldHappenAfter, before)
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

func runTestBackups(client *server.Client, setname, requester string, files []string,
	frequency int, review, remove int64) {
	err := ibackup.Backup(client, setname, requester, files, frequency, review, remove)
	So(err, ShouldBeNil)
}

type mergeCall struct {
	setID string
	paths []string
}
type MockClient struct {
	*server.Client
	Discoveries map[string]time.Time
	MergeCalls  []mergeCall
}

func newMockClient(client *server.Client) *MockClient {
	return &MockClient{
		Client:      client,
		Discoveries: make(map[string]time.Time),
	}
}

func (m *MockClient) TriggerDiscovery(setID string, forceRemovals bool) error {
	m.Discoveries[setID] = time.Now()

	return nil
}

func (m *MockClient) MergeFiles(setID string, paths []string) error {
	m.MergeCalls = append(m.MergeCalls, mergeCall{setID: setID, paths: paths})

	return nil
}
