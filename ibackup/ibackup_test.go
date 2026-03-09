package ibackup_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/ugorji/go/codec"
	"github.com/wtsi-hgi/backup-plans/ibackup"
	ib "github.com/wtsi-hgi/backup-plans/internal/ibackup"
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
		servers := make(map[string]ibackup.ServerDetails)

		_, addr, certPath, dfn, err := ib.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { dfn() }) //nolint:errcheck

		servers["example_1"] = ibackup.ServerDetails{
			Addr:  addr,
			Cert:  certPath,
			Token: filepath.Join(filepath.Dir(certPath), ".ibackup.token"),
		}

		servers["example_2"] = ibackup.ServerDetails{
			FOFNDir: t.TempDir(),
		}

		servers["example_3"] = ibackup.ServerDetails{
			FOFNDir: t.TempDir(),
		}

		config := ibackup.Config{
			Servers: servers,
			PathToServer: map[string]ibackup.ServerTransformer{
				"^/some/path/": {
					ServerName:  "example_1",
					Transformer: ib.CustomTransformer,
				},
				"^/some/other/path/": {
					ServerName:       "example_2",
					ManualServerName: "example_3",
					Transformer:      "prefix=/some/other/path/:/remote/other/path/",
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

		Convey("Connection works when token is in a read-only directory", func() {
			readOnlyDir := t.TempDir()

			firstServerDetails := servers["example_1"]
			tokenContent, err := os.ReadFile(firstServerDetails.Token)
			So(err, ShouldBeNil)

			sharedTokenPath := filepath.Join(readOnlyDir, ".ibackup.token")
			err = os.WriteFile(sharedTokenPath, tokenContent, 0600)
			So(err, ShouldBeNil)

			err = os.Chmod(readOnlyDir, 0555)
			So(err, ShouldBeNil)

			Reset(func() {
				os.Chmod(readOnlyDir, 0755) //nolint:errcheck
			})

			sharedConfig := ibackup.Config{
				Servers: map[string]ibackup.ServerDetails{
					"shared_server": {
						Addr:  firstServerDetails.Addr,
						Cert:  firstServerDetails.Cert,
						Token: sharedTokenPath,
					},
				},
				PathToServer: map[string]ibackup.ServerTransformer{
					"^/shared/": {
						ServerName:  "shared_server",
						Transformer: ib.CustomTransformer,
					},
				},
			}

			mc, err := ibackup.New(sharedConfig)
			So(err, ShouldBeNil)
			So(mc, ShouldNotBeNil)

			mc.Stop()
		})

		Convey("Good config results in a valid MultiClient", func() {
			mc, err := ibackup.New(config)
			So(err, ShouldBeNil)

			u, err := user.Current()
			So(err, ShouldBeNil)

			setName := "mySet"
			setNameB := "myOtherSet"
			setNameC := "myFinalSet"

			Convey("You can backup the same set to different servers", func() {
				So(mc.Backup("/some/path/a/dir/", setName, u.Username,
					[]string{"/some/path/a/dir/file", "/some/path/a/dir/file2"}, 0, true, 1, 2), ShouldBeNil)
				So(mc.Backup("/some/other/path/a/dir/", setName, u.Username,
					[]string{"/some/other/path/a/dir/file"}, 0, true, 3, 4), ShouldBeNil)

				config.PathToServer["^/some/other/path/"] = ibackup.ServerTransformer{
					ServerName:  "example_3",
					Transformer: "prefix=/some/other/path/:/remote/other/path/",
				}

				nc, err := ibackup.New(config)
				So(err, ShouldBeNil)

				So(nc.Backup("/some/other/path/a/dir/", setNameB, u.Username,
					[]string{"/some/other/path/a/dir/file"}, 0, true, 1, 2), ShouldBeNil)
				So(nc.Backup("/some/other/path/another/dir/", setNameC, u.Username,
					[]string{"/some/other/path/another/dir/file"}, 0, true, 1, 2), ShouldBeNil)

				baa, err := mc.GetBackupActivity("/some/path/a/dir/", setName, u.Username, false)
				So(err, ShouldBeNil)

				bac, err := mc.GetBackupActivity("/some/path/a/dir/", setName, u.Username, false)
				So(err, ShouldBeNil)
				So(baa, ShouldEqual, bac)

				bab, err := mc.GetBackupActivity("/some/other/path/a/dir/", setName, u.Username, false)
				So(err, ShouldBeNil)
				So(baa.LastSuccess, ShouldNotEqual, bab.LastSuccess)

				_, err = mc.GetBackupActivity("/some/other/path/another/dir/", setNameC, u.Username, false)
				So(err, ShouldEqual, server.ErrBadSet)

				bad, err := mc.GetBackupActivity("/some/other/path/a/dir/", setNameB, u.Username, true)
				So(err, ShouldBeNil)
				So(bad, ShouldNotEqual, bab)

				_, err = mc.GetBackupActivity("/some/other/path/another/dir/", setNameC, u.Username, true)
				So(err, ShouldBeNil)

				Convey("You can query a MultiCache", func() {
					mcache := ibackup.NewMultiCache(mc, time.Second)

					Reset(mcache.Stop)

					ba, err := mcache.GetBackupActivity("/some/path/a/dir/", setName, u.Username, false)
					So(err, ShouldBeNil)
					So(ba, ShouldResemble, baa)

					for _, client := range *(*map[string]**atomic.Value)(unsafe.Pointer(mc)) {
						*(*string)(reflect.ValueOf((*client).Load()).UnsafePointer()) = ""
					}

					ba, err = mcache.GetBackupActivity("/some/path/a/dir/", setName, u.Username, false)
					So(err, ShouldBeNil)
					So(ba, ShouldResemble, baa)

					ba, err = mcache.GetBackupActivity("/some/other/path/a/dir/", setName, u.Username, false)
					So(err, ShouldNotBeNil)
					So(ba, ShouldBeNil)

					ba, err = mcache.GetBackupActivity("/some/other/path/a/dir/", setNameB, u.Username, true)
					So(err, ShouldBeNil)
					So(ba, ShouldEqual, bad)
				})
			})
		})
	})
}

type ibackupClient interface {
	ibackup.Client
	GetSets(user string) ([]*set.Set, error)
}

func TestIbackup(t *testing.T) {
	ibackupTests(t, func() (ibackupClient, func(*set.Set) error) {
		t.Helper()

		s, addr, certPath, dfn, err := ib.NewTestIbackupServer(t)
		So(err, ShouldBeNil)

		Reset(func() { So(dfn(), ShouldBeNil) })

		client, err := ibackup.Connect(addr, certPath, "")
		So(err, ShouldBeNil)

		return client, func(got *set.Set) error { return setSet(s, got) }
	})
}

func ibackupTests(t *testing.T, createClient func() (ibackupClient, func(*set.Set) error)) {
	t.Helper()

	Convey("Given a new ibackup server", t, func() {
		client, setSet := createClient()

		u, err := user.Current()
		So(err, ShouldBeNil)

		mockClient := newMockClient(client)

		Convey("You can create backup sets", func() {
			sets, err := client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			setName := "mySet"

			err = ibackup.Backup(client, "prefix=/lustre/:/remote/", setName, u.Username, []string{}, 0, true, 0, 0)
			So(err, ShouldBeNil)

			sets, err = client.GetSets(u.Username)
			So(err, ShouldBeNil)
			So(sets, ShouldBeNil)

			before := time.Now()

			review := twoYears.Unix()
			remove := twoYearsAddMonth.Unix()
			setName += "2"

			runTestBackups(client, setName, u.Username,
				[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, false, review, remove)

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

			Convey("You can mark a set as frozen, and change that status later", func() {
				u.Username += "2"

				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, true, 2, 3)

				sets, err = client.GetSets(u.Username)
				So(err, ShouldBeNil)
				So(checkTimes(sets), ShouldResemble, []*set.Set{
					{
						Name:        setName,
						Requester:   u.Username,
						Transformer: "prefix=/lustre/:/remote/",
						Frozen:      true,
						Metadata: map[string]string{
							transfer.MetaKeyReason:  "archive",
							transfer.MetaKeyReview:  timeToMeta(2),
							transfer.MetaKeyRemoval: timeToMeta(3),
						},
						NumFiles: 1,
						Missing:  1,
						Status:   set.Complete,
					},
				})

				ld := time.Now().Add(-24 * time.Hour)
				sets[0].LastDiscovery = ld

				So(setSet(sets[0]), ShouldBeNil)

				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, false, 2, 3)

				sets, err = client.GetSets(u.Username)
				So(err, ShouldBeNil)
				So(checkTimes(sets), ShouldResemble, []*set.Set{
					{
						Name:        setName,
						Requester:   u.Username,
						Transformer: "prefix=/lustre/:/remote/",
						Metadata: map[string]string{
							transfer.MetaKeyReason:  "backup",
							transfer.MetaKeyReview:  timeToMeta(2),
							transfer.MetaKeyRemoval: timeToMeta(3),
						},
						NumFiles: 1,
						Missing:  1,
						Status:   set.Complete,
					},
				})
			})

			Convey("You can create a set with a 0 frequency, which cannot be updated", func() {
				u.Username += "2"

				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 0, true, 0, 0)

				sba, err := ibackup.GetBackupActivity(client, setName, u.Username)
				So(err, ShouldBeNil)
				So(sba.LastSuccess, ShouldNotBeNil)

				err = ibackup.Backup(mockClient, "prefix=/lustre/:/remote/", setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 0, true, 0, 0)

				So(err, ShouldEqual, ibackup.ErrNoUpdate)

				So(len(mockClient.Discoveries), ShouldBeZeroValue)
			})

			Convey("You cannot update a sets files more often than the frequency allows", func() {
				got, err := client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)

				ld := time.Now().Add(-time.Hour)
				got.LastDiscovery = ld

				So(setSet(got), ShouldBeNil)
				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, false, 0, 0)

				got, err = client.GetSetByName(u.Username, setName)
				So(err, ShouldBeNil)
				So(got.LastDiscovery, ShouldEqual, ld)

				ld = time.Now().Add(-24 * time.Hour)
				got.LastDiscovery = ld

				So(setSet(got), ShouldBeNil)

				runTestBackups(client, setName, u.Username,
					[]string{"/lustre/scratch999/humgen/projects/myProject/path/to/a/file"}, 1, false, review, remove)

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

				*(*string)(reflect.ValueOf(client).UnsafePointer()) = ""

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

type fofnClientWrapper struct {
	sets            map[string][]string
	files           map[string]uint64
	lastDiscoveries map[string]time.Time
	*ibackup.FOFNClient
}

func (fc *fofnClientWrapper) GetSetByName(requester, setName string) (*set.Set, error) {
	got, err := fc.FOFNClient.GetSetByName(requester, setName)
	if err != nil {
		return nil, err
	}

	got.StartedDiscovery = fc.lastDiscoveries[got.ID()]
	got.LastCompleted = got.StartedDiscovery
	got.NumFiles = fc.files[got.ID()]
	got.Missing = got.NumFiles
	got.Status = set.Complete

	return got, nil
}

func (fc *fofnClientWrapper) AddOrUpdateSet(set *set.Set) error {
	sets := fc.sets[set.Requester]
	fc.lastDiscoveries[set.ID()] = set.LastDiscovery

	if pos, exists := slices.BinarySearch(sets, set.Name); !exists {
		sets = slices.Insert(sets, pos, set.Name)
	}

	fc.sets[set.Requester] = sets

	return fc.FOFNClient.AddOrUpdateSet(set)
}

func (fc *fofnClientWrapper) MergeFiles(setID string, paths []string) (err error) {
	fc.files[setID] = uint64(len(paths))

	return fc.FOFNClient.MergeFiles(setID, paths)
}

func (fc *fofnClientWrapper) TriggerDiscovery(setID string, forceRemovals bool) error {
	fc.lastDiscoveries[setID] = time.Now()

	err := fc.FOFNClient.TriggerDiscovery(setID, forceRemovals)
	if os.IsNotExist(err) {
		err = nil
	}

	return err
}

func (fc *fofnClientWrapper) GetSets(user string) ([]*set.Set, error) {
	var sets []*set.Set //nolint:prealloc

	for _, setName := range fc.sets[user] {
		got, err := fc.GetSetByName(user, setName)
		if err != nil {
			continue
		}

		sets = append(sets, got)
	}

	return sets, nil
}

func TestIbackupFOFN(t *testing.T) {
	ibackupTests(t, func() (ibackupClient, func(*set.Set) error) {
		t.Helper()

		base := t.TempDir()
		client := ibackup.NewFOFNClient(base)

		wc := &fofnClientWrapper{
			map[string][]string{},
			map[string]uint64{},
			map[string]time.Time{},
			client,
		}

		return wc, wc.AddOrUpdateSet
	})

	Convey("The FOFN creator makes directories group-writable", t, func() {
		base := t.TempDir()
		client := ibackup.NewFOFNClient(base)
		set := &set.Set{
			Name:        "set-name",
			Requester:   "username",
			Transformer: "prefix=/lustre/:/remote/",
		}

		So(client.AddOrUpdateSet(set), ShouldBeNil)

		fi, err := os.Lstat(filepath.Join(base, set.ID()))
		So(err, ShouldBeNil)
		So(fi.Mode(), ShouldEqual, fs.ModeDir|0775)
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

func runTestBackups(client ibackupClient, setname, requester string, files []string,
	frequency int, frozen bool, review, remove int64) {
	err := ibackup.Backup(client, "prefix=/lustre/:/remote/", setname, requester, files, frequency, frozen, review, remove)
	So(err, ShouldBeNil)
}

type mergeCall struct {
	setID string
	paths []string
}
type MockClient struct {
	ibackupClient
	Discoveries map[string]time.Time
	MergeCalls  []mergeCall
}

func newMockClient(client ibackupClient) *MockClient {
	return &MockClient{
		ibackupClient: client,
		Discoveries:   make(map[string]time.Time),
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
