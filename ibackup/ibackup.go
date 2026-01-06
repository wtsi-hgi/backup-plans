package ibackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	Addr, Cert, Token, Username string
}

// ServerTransformer combines a configured server name and a transformer to be
// used.
type ServerTransformer struct {
	ServerName, Transformer string
}

type clientTransformer struct {
	client      *serverClient
	re          *regexp.Regexp
	transformer string
}

// Config contains a map of named ibackup servers, and their connection details,
// and a map of path regexp to server name.
type Config struct {
	Servers      map[string]ServerDetails
	PathToServer map[string]ServerTransformer
}

type UnknownServerError string

func (u UnknownServerError) Error() string {
	return "unknown server name: " + string(u)
}

// MultiClient contains multiple ibackup clients that can be selected by path.
type MultiClient struct {
	clients map[string]*clientTransformer
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

		c, err := connect(
			jwtBasename(details.Token),
			details.Token, details.Addr, details.Cert, details.Username,
		)
		if err != nil {
			continue
		}

		s.Store(c)

		return
	}
}

// New creates a new MultiClient from the given Config.
//
// An error will be returned if any ibackup servers cannot be reached, but a
// MultiClient will also be returned; a goroutine will also be spawned that will
// attempt to establish the connection. The MultiClient.Stop() method should be
// called to stop these goroutines.
//
// The IsOnlyConnectionErrors() function can be used to detect when the error
// returned is only a (potentially temporary) connection error; in such a case,
// it is valid to continue using the returned MultiClient.
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

		c, err := connect(
			jwtBasename(details.Token),
			details.Token, details.Addr, details.Cert, details.Username,
		)
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

// jwtBasename returns a unique JWT filename based on a hash of the token path.
// This ensures JWTs for different servers don't conflict with each other, and
// by returning a non-absolute path, gas.NewClientCLI will store it in TokenDir()
// (the user's home directory), which avoids issues when the token file is in a
// read-only directory shared by another user.
func jwtBasename(tokenPath string) string {
	hash := sha256.Sum256([]byte(tokenPath))

	return ".ibackup." + hex.EncodeToString(hash[:8]) + ".jwt"
}

func createClients(servers map[string]*serverClient, c Config) (map[string]*clientTransformer, error) {
	clients := make(map[string]*clientTransformer, len(c.PathToServer))

	for re, server := range c.PathToServer {
		s, ok := servers[server.ServerName]
		if !ok {
			return nil, UnknownServerError(server.ServerName)
		}

		r, err := regexp.Compile(re)
		if err != nil {
			return nil, err
		}

		clients[re] = &clientTransformer{
			re:          r,
			client:      s,
			transformer: server.Transformer,
		}
	}

	return clients, nil
}

// Stop stops all attempts to connect to previously unreachable ibackup servers.
func (m *MultiClient) Stop() {
	m.stop()
}

// IsOnlyConnectionErrors returns whether the given error only contains
// (potentially temporary) ibackup server connection errors.
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
	for _, c := range m.clients {
		if c.re.MatchString(path) {
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
// the token file next to the cert file. The JWT will be stored in the user's
// XDG_STATE_HOME or home directory.
func Connect(url, cert, username string) (*server.Client, error) {
	tokenPath := filepath.Join(filepath.Dir(cert), ".ibackup.token")

	return connect(jwtBasename(tokenPath), tokenPath, url, cert, username)
}

func connect(jwtBasename, serverTokenBasename, url, cert, username string) (*server.Client, error) {
	client, err := gas.NewClientCLI(jwtBasename, serverTokenBasename, url, cert, false, username)
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

	go cache.runCache(ctx, d)

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

func (c *Cache) runCache(ctx context.Context, d time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}

		c.mu.RLock()
		keys := slices.Collect(maps.Keys(c.cache))
		client := c.client
		c.mu.RUnlock()

		updates := make(map[setRequester]*SetBackupActivity)

		for _, set := range keys {
			ba, err := GetBackupActivity(client, set.set, set.requester)
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

// UpdateClient replaces the store client with the given one, keeping the stored
// cache.
func (c *Cache) UpdateClient(client Client) {
	c.mu.Lock()
	c.client = client
	c.mu.Unlock()
}

// Stop stops the concurrent retrieval of backup statuses.
func (c *Cache) Stop() {
	c.stop()
}

type reCache struct {
	re *regexp.Regexp
	*Cache
}

// MultiCache contains multiple ibackup caches that can be selected by path.
type MultiCache struct {
	d time.Duration

	mu     sync.RWMutex
	caches map[string]reCache
}

// NewMultiCache creates a new MultiClient from the given MultiClient, calling
// NewCache for each client, with the given duration.
//
// The Stop() method must be before replacing (or otherwise losing this pointer
// to) this cache.
func NewMultiCache(mc *MultiClient, d time.Duration) *MultiCache {
	caches := make(map[string]reCache, len(mc.clients))

	for re, c := range mc.clients {
		caches[re] = reCache{re: c.re, Cache: NewCache(c.client, d)}
	}

	return &MultiCache{caches: caches, d: d}
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, c := range m.caches {
		if c.re.MatchString(path) {
			return c.Cache
		}
	}

	return nil
}

// Update replaces the existing MultiClient with the given one, keeping the
// existing caches where possible.
//
// Afterwards, the Stop method on the original MultiClient should be called.
func (m *MultiCache) Update(mc *MultiClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for re, c := range mc.clients {
		if exist, ok := m.caches[re]; ok {
			exist.Cache.UpdateClient(c.client)
		} else {
			m.caches[re] = reCache{re: c.re, Cache: NewCache(c.client, m.d)}
		}
	}

	for re := range m.caches {
		if _, ok := mc.clients[re]; !ok {
			delete(m.caches, re)
		}
	}
}

// Stop stops the concurrent retrieval backup statuses for each client.
func (m *MultiCache) Stop() {
	for _, c := range m.caches {
		c.Stop()
	}
}
