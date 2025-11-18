package ibackup

import (
	"errors"
	"maps"
	"regexp"
	"slices"
	"sync"
	"time"

	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/ibackup/transfer"
)

var (
	isHumgen = regexp.MustCompile(`^/lustre/scratch[0-9]+/humgen/`)
	isGengen = regexp.MustCompile(`^/lustre/scratch[0-9]+/gengen/`)
	isOtar   = regexp.MustCompile(`^/lustre/scratch[0-9]+/open-targets/`)

	ErrInvalidPath = errors.New("cannot determine transformer from path")
	ErrFrozenSet   = errors.New("cannot update frozen backup")
)

// Connect returns a client that can talk to the given ibackup server using
// the .ibackup.jwt and .ibackup.token files.
func Connect(url, cert string) (*server.Client, error) {
	client, err := gas.NewClientCLI(".ibackup.jwt", ".ibackup.token", url, cert, false)
	if err != nil {
		return nil, err
	}

	jwt, err := client.GetJWT()
	if err != nil {
		return nil, err
	}

	return server.NewClient(url, cert, jwt), nil
}

type Client interface {
	GetSetByName(requester, setName string) (*set.Set, error)
	AddOrUpdateSet(set *set.Set) error
	MergeFiles(setID string, paths []string) error
	TriggerDiscovery(setID string, forceRemovals bool) error
}

// Backup creates a new set called setName for the requester if it has been
// longer than the frequency since the last discovery for that set.
func Backup(client Client, setName, requester string, files []string,
	frequency int, review, remove int64) error {
	if len(files) == 0 {
		return nil
	}

	transformer := GetTransformer(files[0])
	if transformer == "" {
		return ErrInvalidPath
	}

	reviewDate := time.Unix(review, 0).Format(time.DateOnly)
	removeDate := time.Unix(remove, 0).Format(time.DateOnly)

	got, err := createOrUpdateSet(client, setName, requester, transformer,
		frequency, reviewDate, removeDate)
	if err != nil {
		return err
	} else if got == nil {
		return nil
	}

	if err := client.MergeFiles(got.ID(), files); err != nil {
		return err
	}

	return client.TriggerDiscovery(got.ID(), false)
}

func createOrUpdateSet(client Client, setName, requester, transformer string,
	frequency int, reviewDate, removeDate string) (*set.Set, error) {
	got, err := client.GetSetByName(requester, setName)
	if errors.Is(err, server.ErrBadSet) {
		return createSet(client, setName, requester, transformer, reviewDate, removeDate, frequency)
	} else if err != nil {
		return nil, err
	}

	if frequency == 0 {
		return got, ErrFrozenSet
	}

	return updateSet(client, got, frequency, reviewDate, removeDate)
}

func createSet(client Client, setName, requester, transformer,
	reviewDate, removeDate string, frequency int) (*set.Set, error) {
	reason := transfer.Backup
	if frequency == 0 {
		reason = transfer.Archive
	}

	m, err := transfer.HandleMeta("", reason, reviewDate, removeDate, nil)
	if err != nil {
		return nil, err
	}

	got := &set.Set{
		Name:        setName,
		Requester:   requester,
		Transformer: transformer,
		Metadata:    m.LocalMeta,
		Failed:      0,
	}

	if err := client.AddOrUpdateSet(got); err != nil {
		return nil, err
	}

	return got, nil
}

func updateSet(client Client, got *set.Set,
	frequency int, reviewDate, removeDate string) (*set.Set, error) {
	if got.LastDiscovery.Add(time.Hour*24*time.Duration(frequency-1) + time.Hour*12).After(time.Now()) { //nolint:nestif
		return nil, nil //nolint:nilnil
	}

	m, err := transfer.HandleMeta("", 0, reviewDate, removeDate, nil)
	if err != nil {
		return nil, err
	}

	if got.Metadata[transfer.MetaKeyReview] != m.LocalMeta[transfer.MetaKeyReview] ||
		got.Metadata[transfer.MetaKeyRemoval] != m.LocalMeta[transfer.MetaKeyRemoval] {
		got.Metadata[transfer.MetaKeyReview] = m.LocalMeta[transfer.MetaKeyReview]
		got.Metadata[transfer.MetaKeyRemoval] = m.LocalMeta[transfer.MetaKeyRemoval]

		if err := client.AddOrUpdateSet(got); err != nil {
			return nil, err
		}
	}

	return got, nil
}

// GetTransformer returns the named transformer for the path given, returning
// empty string when a transformer cannot be automatically determined.
func GetTransformer(file string) string {
	if isHumgen.MatchString(file) {
		return "humgen"
	}

	if isGengen.MatchString(file) {
		return "gengen"
	}

	if isOtar.MatchString(file) {
		return "otar"
	}

	return ""
}

// SetBackupActivity holds info about backup activity retrieved from an ibackup
// server.
type SetBackupActivity struct {
	LastSuccess time.Time
	Name        string
	Requester   string
	Failures    uint64
}

// GetBackupActivity queries an ibackup server to get the last completed backup
// date and number of failures for the given set name and requester.
func GetBackupActivity(client *server.Client, setName, requester string) (*SetBackupActivity, error) {
	var (
		sba SetBackupActivity
		err error
	)

	sba.Name = setName
	sba.Requester = requester

	got, err := client.GetSetByName(requester, setName)
	if err != nil {
		return nil, err
	}

	if got != nil {
		sba.Failures = got.Failed
		sba.LastSuccess = got.LastCompleted
	}

	return &sba, nil
}

type setRequester struct {
	set, requester string
}

type Cache struct {
	client   *server.Client
	duration time.Duration

	mu    sync.RWMutex
	cache map[setRequester]*SetBackupActivity
}

func NewCache(client *server.Client, d time.Duration) *Cache {
	cache := &Cache{
		client:   client,
		duration: d,
		cache:    make(map[setRequester]*SetBackupActivity),
	}

	go cache.runCache()

	return cache
}

func (c *Cache) GetBackupActivity(setName, requester string) (*SetBackupActivity, error) {
	sr := setRequester{set: setName, requester: requester}

	c.mu.RLock()
	existing, ok := c.cache[sr]
	c.mu.RUnlock()

	if ok {
		return existing, nil
	}

	sba, err := GetBackupActivity(c.client, setName, requester)

	c.mu.Lock()
	c.cache[sr] = sba
	c.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return sba, nil
}

func (c *Cache) runCache() {
	for {
		time.Sleep(c.duration)

		c.mu.RLock()
		keys := slices.Collect(maps.Keys(c.cache))
		c.mu.RUnlock()

		updates := make(map[setRequester]*SetBackupActivity)

		for _, set := range keys {
			ba, err := GetBackupActivity(c.client, set.set, set.requester)
			if err != nil {
				continue
			}

			updates[set] = ba
		}

		c.mu.Lock()

		for set, ba := range updates {
			c.cache[set] = ba
		}

		c.mu.Unlock()
	}
}
