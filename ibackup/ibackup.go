/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
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

package ibackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	gas "github.com/wtsi-hgi/go-authserver"
	"github.com/wtsi-hgi/ibackup/fofn"
	"github.com/wtsi-hgi/ibackup/server"
	"github.com/wtsi-hgi/ibackup/set"
	"github.com/wtsi-hgi/ibackup/transfer"
)

var (
	ErrInvalidPath   = errors.New("cannot determine transformer from path")
	ErrUnknownClient = errors.New("cannot determine client from path")
	ErrFrozenSet     = errors.New("cannot update frozen backup")
	errNoAddrOrFofn  = errors.New("server requires addr or fofndir")
)

// ServerDetails contains the connection details for a particular ibackup
// server.
type ServerDetails struct {
	Addr, Cert, Token, Username string
	FofnDir                     string
}

// ServerTransformer combines a configured server name and a transformer to be
// used.
type ServerTransformer struct {
	ServerName, Transformer string
}

type serverClient struct {
	atomic.Pointer[server.Client]
}

func createServerClient(ctx context.Context, details ServerDetails) (*serverClient, error) {
	var client serverClient

	c, err := connect(
		jwtBasename(details.Token),
		details.Token, details.Addr, details.Cert, details.Username,
	)
	if err != nil {
		client.Store(new(server.Client))

		go client.await(ctx, details)

		return &client, err
	}

	client.Store(c)

	return &client, nil
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

// jwtBasename returns a unique JWT filename based on a hash of the token path.
// This ensures JWTs for different servers don't conflict with each other, and
// by returning a non-absolute path, gas.NewClientCLI will store it in TokenDir()
// (the user's home directory), which avoids issues when the token file is in a
// read-only directory shared by another user.
func jwtBasename(tokenPath string) string {
	hash := sha256.Sum256([]byte(tokenPath))

	return ".ibackup." + hex.EncodeToString(hash[:8]) + ".jwt"
}

// Backup creates a new set called setName for the requester if it has been
// longer than the frequency since the last discovery for that set.
func Backup(client Client, transformer, setName, requester string, files []string,
	frequency uint, review, remove int64) error {
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
	frequency uint, reviewDate, removeDate string) (*set.Set, error) {
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
	reviewDate, removeDate string, frequency uint) (*set.Set, error) {
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
	frequency uint, reviewDate, removeDate string) (*set.Set, error) {
	deadline := time.Hour*24*time.Duration(frequency-1) + //nolint:gosec // G115: frequency is small
		time.Hour*frequencyWindowOffsetHours

	if got.LastDiscovery.Add(deadline).After(time.Now()) {
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

func createServers(ctx context.Context, c Config) (map[string]*serverClient, error) {
	var errs error

	servers := make(map[string]*serverClient, len(c.Servers))

	for name, details := range c.Servers {
		if details.Addr == "" {
			if details.FofnDir != "" {
				continue
			}

			errs = errors.Join(errs, &ServerConnectionError{name, errNoAddrOrFofn})

			continue
		}

		client, err := createServerClient(ctx, details)
		if err != nil {
			errs = errors.Join(errs, &ServerConnectionError{name, err})
		}

		servers[name] = client
	}

	return servers, errs
}

func createClients(servers map[string]*serverClient, c Config) (map[string]*clientTransformer, error) {
	clients := make(map[string]*clientTransformer, len(c.PathToServer))

	for re, server := range c.PathToServer {
		ct, err := buildClientTransformer(servers, c.Servers, re, server)
		if err != nil {
			return nil, err
		}

		clients[re] = ct
	}

	return clients, nil
}

func buildClientTransformer(servers map[string]*serverClient,
	serverDetails map[string]ServerDetails, re string, server ServerTransformer,
) (*clientTransformer, error) {
	details, ok := serverDetails[server.ServerName]
	if !ok {
		return nil, UnknownServerError(server.ServerName)
	}

	var s *serverClient

	if details.Addr != "" {
		s, ok = servers[server.ServerName]
		if !ok {
			return nil, UnknownServerError(server.ServerName)
		}
	}

	r, err := regexp.Compile(re)
	if err != nil {
		return nil, err
	}

	return &clientTransformer{
		re:          r,
		client:      s,
		transformer: server.Transformer,
		fofnDir:     details.FofnDir,
	}, nil
}

type clientTransformer struct {
	client      *serverClient
	re          *regexp.Regexp
	transformer string
	fofnDir     string
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

// SetBackupActivity holds info about backup activity retrieved from an ibackup
// server.
type SetBackupActivity struct {
	LastSuccess time.Time
	Name        string
	Requester   string
	Failures    uint64
	Uploaded    int `json:",omitempty"`
	Replaced    int `json:",omitempty"`
	Unmodified  int `json:",omitempty"`
	Missing     int `json:",omitempty"`
	Frozen      int `json:",omitempty"`
	Orphaned    int `json:",omitempty"`
	Warning     int `json:",omitempty"`
	Hardlink    int `json:",omitempty"`
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

// SetWriter accumulates file paths for a backup set and routes them to
// the appropriate destination (fofn file, API, or both). For fofn-backed
// servers, paths are streamed directly to the fofn file during Add calls.
// For API-backed servers, paths are collected in memory.
//
// Call Finish after adding all paths to complete the backup (close fofn,
// write config, call API). Call Close to release resources on error.
type SetWriter struct {
	c         *clientTransformer
	setName   string
	requester string
	frequency uint
	review    int64
	remove    int64

	fofnDir     string
	fofnSubDir  string
	fofnFD      *os.File
	fofnSkipped bool
	fofnWrote   bool

	files    []string
	count    int
	err      error
	finished bool
}

// Add appends a file path to this set. For fofn destinations the path
// is written directly to disk; for API destinations it is collected in
// memory.
func (sw *SetWriter) Add(filePath string) error {
	if sw.err != nil {
		return sw.err
	}

	sw.count++

	if sw.fofnDir != "" && !sw.fofnSkipped {
		if err := sw.writeFofnPath(filePath); err != nil {
			sw.err = err

			return err
		}
	}

	if sw.c.client != nil {
		sw.files = append(sw.files, filePath)
	}

	return nil
}

func (sw *SetWriter) writeFofnPath(filePath string) error {
	if sw.fofnFD == nil {
		fd, err := createFofnFile(sw.fofnSubDir)
		if err != nil {
			return err
		}

		sw.fofnFD = fd
	}

	err := writePath(sw.fofnFD, filePath)
	if err != nil {
		sw.fofnFD.Close()
		sw.fofnFD = nil

		return err
	}

	sw.fofnWrote = true

	return nil
}

// Count returns the number of paths added.
func (sw *SetWriter) Count() int {
	return sw.count
}

// Finish completes the backup: closes the fofn file, writes config,
// and submits to the API server. It must be called exactly once.
func (sw *SetWriter) Finish() error {
	if sw.finished {
		return nil
	}

	sw.finished = true

	if sw.err != nil {
		sw.Close()

		return sw.err
	}

	if err := sw.finishFofn(); err != nil {
		sw.Close()

		return err
	}

	if sw.c.client == nil || len(sw.files) == 0 {
		return nil
	}

	return Backup(sw.c.client.Load(), sw.c.transformer, sw.setName,
		sw.requester, sw.files, sw.frequency, sw.review, sw.remove)
}

func (sw *SetWriter) fofnMetadata() map[string]string {
	return map[string]string{
		"requestor": sw.requester,
		"review":    time.Unix(sw.review, 0).Format(time.DateOnly),
		"remove":    time.Unix(sw.remove, 0).Format(time.DateOnly),
	}
}

func (sw *SetWriter) finishFofn() error {
	if sw.fofnDir == "" {
		return nil
	}

	if sw.fofnFD != nil {
		if err := sw.fofnFD.Close(); err != nil {
			return err
		}

		sw.fofnFD = nil
	}

	metadata := sw.fofnMetadata()

	if sw.fofnWrote {
		return writeConfig(sw.fofnSubDir, sw.c.transformer,
			sw.frequency == 0, metadata)
	}

	changed, err := requestorChanged(sw.fofnDir, sw.setName,
		sw.requester)
	if err != nil || !changed {
		return err
	}

	return NewFofnDirWriter(sw.fofnDir).UpdateConfig(sw.setName,
		sw.c.transformer, sw.frequency == 0, metadata)
}

func requestorChanged(fofnDir, setName, requestor string) (bool, error) {
	config, err := fofn.ReadConfig(filepath.Join(fofnDir, SafeName(setName)))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	return config.Metadata["requestor"] != requestor, nil
}

// Close releases resources. It is safe to call multiple times.
func (sw *SetWriter) Close() {
	if sw.fofnFD != nil {
		sw.fofnFD.Close()
		sw.fofnFD = nil
	}
}

// MultiClient contains multiple ibackup clients that can be selected by path.
type MultiClient struct {
	clients map[string]*clientTransformer
	stop    func()
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

// Stop stops all attempts to connect to previously unreachable ibackup servers.
func (m *MultiClient) Stop() {
	m.stop()
}

// Backup retrieves a client using the given path and backs up the files.
// For fofn-backed servers the files are streamed directly to the fofn
// file; for API-backed servers they are passed as a slice.
func (m *MultiClient) Backup(path string, setName, requester string, files []string,
	frequency uint, review, remove int64) error {
	w, err := m.NewSetWriter(path, setName, requester, frequency, review, remove)
	if err != nil {
		return err
	}

	defer w.Close()

	for _, f := range files {
		if err := w.Add(f); err != nil {
			return err
		}
	}

	return w.Finish()
}

// NewSetWriter creates a writer for the backup set matching the given
// path. The writer streams paths directly to fofn (when configured)
// and collects them in memory for the API (when an API server exists).
func (m *MultiClient) NewSetWriter(path, setName, requester string,
	frequency uint, review, remove int64) (*SetWriter, error) {
	c := m.getClient(path)
	if c == nil {
		return nil, ErrUnknownClient
	}

	sw := &SetWriter{
		c:         c,
		setName:   setName,
		requester: requester,
		frequency: frequency,
		review:    review,
		remove:    remove,
	}

	if c.fofnDir != "" {
		sw.fofnDir = c.fofnDir
		sw.fofnSubDir = filepath.Join(c.fofnDir, SafeName(setName))

		writer := NewFofnDirWriter(c.fofnDir)

		shouldWrite, err := writer.shouldWrite(setName, frequency)
		if err != nil {
			return nil, err
		}

		sw.fofnSkipped = !shouldWrite
	}

	return sw, nil
}

// GetFofnDir returns the fofn directory configured for the server matching the
// given path, or "" if none is configured.
func (m *MultiClient) GetFofnDir(path string) string {
	c := m.getClient(path)
	if c == nil {
		return ""
	}

	return c.fofnDir
}

// GetTransformer returns the transformer string configured for the server
// matching the given path, or "" if none matches.
func (m *MultiClient) GetTransformer(path string) string {
	c := m.getClient(path)
	if c == nil {
		return ""
	}

	return c.transformer
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

	if c.client == nil {
		return nil, ErrUnknownClient
	}

	return GetBackupActivity(c.client, setName, requester)
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

// Client represents the required methods for an ibackup client.
type Client interface {
	GetSetByName(requester, setName string) (*set.Set, error)
	AddOrUpdateSet(set *set.Set) error
	MergeFiles(setID string, paths []string) error
	TriggerDiscovery(setID string, forceRemovals bool) error
}

// Connect returns a client that can talk to the given ibackup server using
// the token file next to the cert file. The JWT will be stored in the user's
// XDG_STATE_HOME or home directory.
func Connect(url, cert, username string) (*server.Client, error) {
	tokenPath := filepath.Join(filepath.Dir(cert), ".ibackup.token")

	return connect(jwtBasename(tokenPath), tokenPath, url, cert, username)
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

	if d > 0 {
		go cache.runCache(ctx, d)
	}

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

type serverCache interface {
	Match(path string) bool
	GetBackupActivity(setName, requester string) (*SetBackupActivity, error)
	GetFofnBackupActivity(setName string) (*SetBackupActivity, error)
	Update(c *clientTransformer, d time.Duration)
	Stop()
}

// newServerCache creates the appropriate serverCache
// based on whether c has an API client.
func newServerCache(c *clientTransformer, d time.Duration) serverCache {
	if c.client != nil {
		return newAPIServerCache(c, d)
	}

	return newFofnOnlyCache(c, d)
}

// apiServerCache is a serverCache backed by an API client,
// optionally supplemented by fofn status.
type apiServerCache struct {
	re   *regexp.Regexp
	main *Cache
	fofn *fofnCache
}

func newAPIServerCache(c *clientTransformer, d time.Duration) *apiServerCache {
	ac := &apiServerCache{
		re:   c.re,
		main: NewCache(c.client, d),
	}

	ac.updateFofnCache(c, d)

	return ac
}

func (r *apiServerCache) Match(path string) bool {
	return r.re.MatchString(path)
}

func (r *apiServerCache) GetBackupActivity(
	setName, requester string,
) (*SetBackupActivity, error) {
	return r.main.GetBackupActivity(setName, requester)
}

func (r *apiServerCache) GetFofnBackupActivity(
	setName string,
) (*SetBackupActivity, error) {
	if r.fofn == nil {
		return nil, nil //nolint:nilnil
	}

	return r.fofn.GetBackupActivity(setName)
}

func (r *apiServerCache) Update(c *clientTransformer, d time.Duration) {
	r.updateMainCache(c, d)
	r.updateFofnCache(c, d)
}

func (r *apiServerCache) updateMainCache(c *clientTransformer, _ time.Duration) {
	if c.client == nil {
		return
	}

	r.main.UpdateClient(c.client)
}

func (r *apiServerCache) updateFofnCache(c *clientTransformer, d time.Duration) {
	if c.fofnDir == "" {
		if r.fofn != nil {
			r.fofn.Stop()
			r.fofn = nil
		}

		return
	}

	if r.fofn == nil || r.fofn.reader.baseDir != c.fofnDir {
		if r.fofn != nil {
			r.fofn.Stop()
		}

		r.fofn = newFofnCache(NewFofnStatusReader(c.fofnDir), d)
	}
}

func (r *apiServerCache) Stop() {
	r.main.Stop()

	if r.fofn != nil {
		r.fofn.Stop()
	}
}

// fofnOnlyCache is a serverCache backed only by fofn
// status, with no API client.
type fofnOnlyCache struct {
	re   *regexp.Regexp
	fofn *fofnCache
}

func newFofnOnlyCache(c *clientTransformer, d time.Duration) *fofnOnlyCache {
	fc := &fofnOnlyCache{re: c.re}
	fc.updateFofnCache(c, d)

	return fc
}

func (r *fofnOnlyCache) Match(path string) bool {
	return r.re.MatchString(path)
}

func (r *fofnOnlyCache) GetBackupActivity(string, string) (*SetBackupActivity, error) {
	return nil, ErrUnknownClient
}

func (r *fofnOnlyCache) GetFofnBackupActivity(
	setName string,
) (*SetBackupActivity, error) {
	if r.fofn == nil {
		return nil, nil //nolint:nilnil
	}

	return r.fofn.GetBackupActivity(setName)
}

func (r *fofnOnlyCache) Update(c *clientTransformer, d time.Duration) {
	r.updateFofnCache(c, d)
}

func (r *fofnOnlyCache) updateFofnCache(c *clientTransformer, d time.Duration) {
	if c.fofnDir == "" {
		if r.fofn != nil {
			r.fofn.Stop()
			r.fofn = nil
		}

		return
	}

	if r.fofn == nil || r.fofn.reader.baseDir != c.fofnDir {
		if r.fofn != nil {
			r.fofn.Stop()
		}

		r.fofn = newFofnCache(NewFofnStatusReader(c.fofnDir), d)
	}
}

func (r *fofnOnlyCache) Stop() {
	if r.fofn != nil {
		r.fofn.Stop()
	}
}

// MultiCache contains multiple ibackup caches that can be selected by path.
type MultiCache struct {
	d time.Duration

	mu     sync.RWMutex
	caches map[string]serverCache
}

// NewMultiCache creates a new MultiClient from the given MultiClient, calling
// NewCache for each client, with the given duration.
//
// The Stop() method must be before replacing (or otherwise losing this pointer
// to) this cache.
func NewMultiCache(mc *MultiClient, d time.Duration) *MultiCache {
	caches := make(map[string]serverCache, len(mc.clients))

	for re, c := range mc.clients {
		caches[re] = newServerCache(c, d)
	}

	return &MultiCache{caches: caches, d: d}
}

// GetBackupActivity retrieves a cache using the given path, and then calls the
// normal GetBackupActivity method.
func (m *MultiCache) GetBackupActivity(path, setName, requester string) (*SetBackupActivity, error) {
	c := m.getCache(path)
	if c == nil {
		return nil, ErrUnknownClient
	}

	return c.GetBackupActivity(setName, requester)
}

// GetFofnBackupActivity retrieves fofn-based backup status for the given path
// and set name. Returns nil if no fofndir is configured for the path or no
// status file exists.
func (m *MultiCache) GetFofnBackupActivity(path, setName string) (*SetBackupActivity, error) {
	c := m.getCache(path)
	if c == nil {
		return nil, nil //nolint:nilnil
	}

	return c.GetFofnBackupActivity(setName)
}

func (m *MultiCache) getCache(path string) serverCache {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, c := range m.caches {
		if c.Match(path) {
			return c
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
		exist, ok := m.caches[re]
		if ok {
			exist.Update(c, m.d)

			continue
		}

		m.caches[re] = newServerCache(c, m.d)
	}

	for re, cache := range m.caches {
		if _, ok := mc.clients[re]; ok {
			continue
		}

		cache.Stop()
		delete(m.caches, re)
	}
}

// Stop stops the concurrent retrieval backup statuses for each client.
func (m *MultiCache) Stop() {
	for _, c := range m.caches {
		c.Stop()
	}
}
