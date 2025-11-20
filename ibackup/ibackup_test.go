package ibackup_test

import (
	"errors"
	"io"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"sync/atomic"
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
	twoYears         = time.Now().Add(time.Hour * 24 * 365 * 2) //nolint:gochecknoglobals
	twoYearsAddMonth = twoYears.Add(time.Hour * 24 * 30)        //nolint:gochecknoglobals
)

func TestMultiIbackup(t *testing.T) {
	Convey("Given multiple ibackup servers", t, func() {
		So(testirods.AddPseudoIRODsToolsToPathIfRequired(t), ShouldBeNil)

		servers := make(map[string]ibackup.ServerDetails)

		for i := range 2 {
			_, addr, certPath, dfn, err := ib.NewTestIbackupServer(t)
			So(err, ShouldBeNil)

			Reset(func() { dfn() })

			servers["example_"+strconv.Itoa(i+1)] = ibackup.ServerDetails{
				Addr:  addr,
				Cert:  certPath,
				Token: filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
			}
		}

		config := ibackup.Config{
			Servers: servers,
			PathToServer: map[string]ibackup.ServerTransformer{
				"^/some/path/": {
					ServerName:  "example_1",
					Transformer: ib.CustomTransformer,
				},
				"^/some/other/path/": {
					ServerName:  "example_2",
					Transformer: "prefix=/some/other/path/:/remote/other/path/",
				},
			},
		}

		Convey("Bad config results in errors", func() {
			servers["not_a_running_server"] = ibackup.ServerDetails{}

			mc, err := ibackup.New(config)
			So(mc, ShouldNotBeNil)
			So(err, ShouldNotBeNil)
			So(ibackup.IsOnlyConnectionErrors(err), ShouldBeTrue)

			servers["another_non-running_server"] = ibackup.ServerDetails{}

			mc, err = ibackup.New(config)
			So(mc, ShouldNotBeNil)
			So(err, ShouldNotBeNil)
			So(ibackup.IsOnlyConnectionErrors(err), ShouldBeTrue)

			So(ibackup.IsOnlyConnectionErrors(errors.Join(err, io.EOF)), ShouldBeFalse)

			config.PathToServer["["] = ibackup.ServerTransformer{
				ServerName:  "example_1",
				Transformer: ib.CustomTransformer,
			}

			delete(servers, "not_a_running_server")
			delete(servers, "another_non-running_server")

			mc, err = ibackup.New(config)
			So(mc, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(ibackup.IsOnlyConnectionErrors(err), ShouldBeFalse)

			delete(config.PathToServer, "[")

			config.PathToServer["^/a/path/"] = ibackup.ServerTransformer{
				ServerName:  "not_a_server",
				Transformer: ib.CustomTransformer,
			}

			mc, err = ibackup.New(config)
			So(mc, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(ibackup.IsOnlyConnectionErrors(err), ShouldBeFalse)
		})

		Convey("Good config results in a valid MultiClient", func() {
			mc, err := ibackup.New(config) // Connect to the servers and store, but don't stop on server error; DO stop on regex error
			So(err, ShouldBeNil)

			u, err := user.Current()
			So(err, ShouldBeNil)

			setName := "mySet"

			Convey("You can backup the same set to different servers", func() {
				So(mc.Backup("/some/path/a/dir/", setName, u.Username, []string{"/some/path/a/dir/file", "/some/path/a/dir/file2"}, 0, 1, 2), ShouldBeNil)
				So(mc.Backup("/some/other/path/a/dir/", setName, u.Username, []string{"/some/other/path/a/dir/file"}, 0, 3, 4), ShouldBeNil)

				baa, err := mc.GetBackupActivity("/some/path/a/dir/", setName, u.Username)
				So(err, ShouldBeNil)

				bab, err := mc.GetBackupActivity("/some/other/path/a/dir/", setName, u.Username)
				So(err, ShouldBeNil)

				So(baa.LastSuccess, ShouldNotEqual, bab.LastSuccess)

				Convey("You can query a MultiCache", func() {
					mcache := ibackup.NewMultiCache(mc, time.Second)

					Reset(mcache.Stop)

					ba, err := mcache.GetBackupActivity("/some/path/a/dir/", setName, u.Username)
					So(err, ShouldBeNil)
					So(ba, ShouldResemble, baa)

					for _, client := range *(*map[*regexp.Regexp]**atomic.Pointer[server.Client])(unsafe.Pointer(mc)) {
						*(*string)(unsafe.Pointer((*client).Load())) = ""
					}

					ba, err = mcache.GetBackupActivity("/some/path/a/dir/", setName, u.Username)
					So(err, ShouldBeNil)
					So(ba, ShouldResemble, baa)

					ba, err = mcache.GetBackupActivity("/some/other/path/a/dir/", setName, u.Username)
					So(err, ShouldNotBeNil)
					So(ba, ShouldBeNil)
				})
			})
		})
	})
}

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

			err = ibackup.Backup(client, "prefix=/lustre/:/remote/", setName, u.Username, []string{}, 0, 0, 0)
			So(err, ShouldBeNil)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			before := time.Now()

			review := twoYears.Unix()
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
					Transformer: "prefix=/lustre/:/remote/",
					Metadata: map[string]string{
						transfer.MetaKeyReason:  "backup",
						transfer.MetaKeyReview:  timeToMeta(review),
						transfer.MetaKeyRemoval: timeToMeta(remove),
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

				err = ibackup.Backup(mockClient, "prefix=/lustre/:/remote/", setName, u.Username,
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
				manualSet := &set.Set{
					Name:        manualSetName,
					Requester:   u.Username,
					Transformer: "prefix=/lustre/:/remote/",
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
	err := ibackup.Backup(client, "prefix=/lustre/:/remote/", setname, requester, files, frequency, review, remove)
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

// timeToMeta converts a time to a string suitable for storing as metadata, in
// a way that ObjectInfo.ModTime() will understand and be able to convert back
// again.
func timeToMeta(t int64) string {
	b, _ := time.Unix(t, 0).UTC().Truncate(24 * time.Hour).MarshalText() //nolint:errcheck

	return string(b)
}
