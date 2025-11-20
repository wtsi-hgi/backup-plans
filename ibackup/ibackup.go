package ibackup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/ibackup/transfer"
)

var (
	ErrInvalidPath   = errors.New("cannot determine transformer from path")
	ErrUnknownClient = errors.New("cannot determine client from path")
	ErrFrozenSet     = errors.New("cannot update frozen backup")
)

// ServerDetails contains the connection details for a particular ibackup
// server.
type ServerDetails struct {
	Addr, Cert, Token string
}

type ServerTransformer struct {
	ServerName, Transformer string
}

type clientTransformer struct {
	client      *serverClient
	transformer string
}

// Config contains a map of named ibackup servers, and their connection details,
// and a map of path regexp to server name.
type Config struct {
	Servers      map[string]ServerDetails
	PathToServer map[string]ServerTransformer
}

// MultiClient contains multiple ibackup clients that can be selected by path.
type MultiClient struct {
	clients map[*regexp.Regexp]*clientTransformer
	stop    func()
}

type ServerConnectionError struct {
	server string
	err    error
}

func (s ServerConnectionError) Error() string {
	return s.server + ": " + s.err.Error()
}

func (s ServerConnectionError) Unwrap() error {
	return s.err
}

type serverClient struct {
	atomic.Pointer[server.Client]
}

func (s *serverClient) GetSetByName(requester, setName string) (*set.Set, error) {
	return s.Load().GetSetByName(requester, setName)
}

func (s *serverClient) AddOrUpdateSet(set *set.Set) error {
	return s.Load().AddOrUpdateSet(set)
}

func (s *serverClient) MergeFiles(setID string, paths []string) error {
	return s.Load().MergeFiles(setID, paths)
}

func (s *serverClient) TriggerDiscovery(setID string, forceRemovals bool) error {
	return s.Load().TriggerDiscovery(setID, forceRemovals)
}

func (s *serverClient) await(ctx context.Context, details ServerDetails) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}

		c, err := connect(details.Token+".jwt", details.Token, details.Addr, details.Cert)
		if err != nil {
			continue
		}

		s.Store(c)

		return
	}
}

// New creates a new MultiClient from the given Config.
func New(c Config) (*MultiClient, error) {
	ctx, stop := context.WithCancel(context.Background())

	servers, errs := createServers(ctx, c)

	clients, err := createClients(servers, c)
	if err != nil {
		stop()

		return nil, err
	}

	return &MultiClient{clients: clients, stop: stop}, errs
}

func createServers(ctx context.Context, c Config) (map[string]*serverClient, error) {
	var errs error

	servers := make(map[string]*serverClient, len(c.Servers))

	for name, details := range c.Servers {
		var client serverClient

		c, err := connect(details.Token+".jwt", details.Token, details.Addr, details.Cert)
		if err != nil {
			errs = errors.Join(errs, &ServerConnectionError{name, err})

			client.Store(new(server.Client))

			go client.await(ctx, details)
		} else {
			client.Store(c)
		}

		servers[name] = &client
	}

	return servers, errs
}

func createClients(servers map[string]*serverClient, c Config) (map[*regexp.Regexp]*clientTransformer, error) {
	clients := make(map[*regexp.Regexp]*clientTransformer, len(c.PathToServer))

	for re, server := range c.PathToServer {
		s, ok := servers[server.ServerName]
		if !ok {
			return nil, fmt.Errorf("unknown server name: %q", server.ServerName)
		}

		r, err := regexp.Compile(re)
		if err != nil {
			return nil, err
		}

		clients[r] = &clientTransformer{
			client:      s,
			transformer: server.Transformer,
		}
	}

	return clients, nil
}

func (m *MultiClient) Stop() {
	m.stop()
}

func IsOnlyConnectionErrors(err error) bool {
	var sce *ServerConnectionError

	errI, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return errors.As(err, &sce)
	}

	for _, err := range errI.Unwrap() {
		if !errors.As(err, &sce) {
			return false
		}
	}

	return true
}

// Backup retrieves a client using the given path, and then calls the normal
// Backup function.
func (m *MultiClient) Backup(path string, setName, requester string, files []string,
	frequency int, review, remove int64) error {
	c := m.getClient(path)
	if c == nil {
		return ErrUnknownClient
	}

	return Backup(c.client.Load(), c.transformer, setName, requester, files, frequency, review, remove)
}

func (m *MultiClient) getClient(path string) *clientTransformer {
	for re, c := range m.clients {
		if re.MatchString(path) {
			return c
		}
	}

	return nil
}

// GetBackupActivity retrieves a client using the given path, and then calls the
// normal GetBackupActivity function.
func (m *MultiClient) GetBackupActivity(path, setName, requester string) (*SetBackupActivity, error) {
	c := m.getClient(path)
	if c == nil {
		return nil, ErrInvalidPath
	}

	return GetBackupActivity(c.client, setName, requester)
}

// Connect returns a client that can talk to the given ibackup server using
// the .ibackup.jwt and .ibackup.token files.
func Connect(url, cert string) (*server.Client, error) {
	base := filepath.Dir(cert)

	return connect(
		filepath.Join(base, ".ibackup.jwt"),
		filepath.Join(base, ".ibackup.token"),
		url, cert,
	)
}

func connect(jwtBasename, serverTokenBasename, url, cert string) (*server.Client, error) {
	client, err := gas.NewClientCLI(jwtBasename, serverTokenBasename, url, cert, false)
	if err != nil {
		return nil, err
	}

	jwt, err := client.GetJWT()
	if err != nil {
		return nil, err
	}

	slog.Info("ibackup server connected", "url", url)

	return server.NewClient(url, cert, jwt), nil
}

// Client represents the required methods for an ibackup client.
type Client interface {
	GetSetByName(requester, setName string) (*set.Set, error)
	AddOrUpdateSet(set *set.Set) error
	MergeFiles(setID string, paths []string) error
	TriggerDiscovery(setID string, forceRemovals bool) error
}

// Backup creates a new set called setName for the requester if it has been
// longer than the frequency since the last discovery for that set.
func Backup(client Client, transformer, setName, requester string, files []string,
	frequency int, review, remove int64) error {
	if len(files) == 0 {
		return nil
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
	if got.LastDiscovery.Add(time.Hour*24*time.Duration(frequency-1) + time.Hour*12).After(time.Now()) {
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
func GetBackupActivity(client Client, setName, requester string) (*SetBackupActivity, error) {
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

// Cache wraps the GetBackupActivity method for an ibackup client, caching, and
// on a schedule re-retrieving, requested set backup information.
type Cache struct {
	client Client
	stop   func()

	mu    sync.RWMutex
	cache map[setRequester]*SetBackupActivity
}

// NewCache creates a cache for a particular ibackup client, that will
// re-retrieve set backup information on a timeout specified by the given
// Duration.
//
// The Stop() method must be before replacing (or otherwise losing this pointer
// to) this cache.
func NewCache(client Client, d time.Duration) *Cache {
	ctx, fn := context.WithCancel(context.Background())

	cache := &Cache{
		client: client,
		cache:  make(map[setRequester]*SetBackupActivity),
		stop:   fn,
	}

	go cache.runCache(d, ctx)

	return cache
}

// GetBackupActivity acts like the GetBackupActivity function, but looks in its
// cache for the information before retrieving from the stored client.
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

func (c *Cache) runCache(d time.Duration, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}

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
		maps.Copy(c.cache, updates)
		c.mu.Unlock()
	}
}

// Stop stops the concurrent retrieval of backup statuses.
func (c *Cache) Stop() {
	c.stop()
}

// MultiCache contains multiple ibackup caches that can be selected by path.
type MultiCache struct {
	caches map[*regexp.Regexp]*Cache
}

// NewMultiCache creates a new MultiClient from the given MultiClient, calling
// NewCache for each client, with the given duration.
//
// The Stop() method must be before replacing (or otherwise losing this pointer
// to) this cache.
func NewMultiCache(mc *MultiClient, d time.Duration) *MultiCache {
	caches := make(map[*regexp.Regexp]*Cache, len(mc.clients))

	for re, c := range mc.clients {
		caches[re] = NewCache(c.client, d)
	}

	return &MultiCache{caches: caches}
}

// GetBackupActivity retrieves a cache using the given path, and then calls the
// normal GetBackupActivity method.
func (m *MultiCache) GetBackupActivity(path, setName, requester string) (*SetBackupActivity, error) {
	c := m.getClient(path)
	if c == nil {
		return nil, ErrUnknownClient
	}

	return c.GetBackupActivity(setName, requester)
}

func (m *MultiCache) getClient(path string) *Cache {
	for re, c := range m.caches {
		if re.MatchString(path) {
			return c
		}
	}

	return nil
}

// Stop stops the concurrent retrieval backup statuses for each client.
func (m *MultiCache) Stop() {
	for _, c := range m.caches {
		c.Stop()
	}
}
