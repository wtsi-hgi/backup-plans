package git

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/storage/memory"
)

// GetLatestCommitDate returns the commit date for the HEAD of the supplied
// repo.
func GetLatestCommitDate(url string) (time.Time, error) {
	repo, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:    url,
		Depth:  1,
		Bare:   true,
		Filter: packp.FilterBlobNone(),
	})
	if errors.Is(err, transport.ErrFilterNotSupported) {
		repo, err = git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
			URL:   url,
			Depth: 1,
			Bare:  true,
		})
	}

	if err != nil {
		return time.Time{}, err
	}

	head, err := repo.Head()
	if err != nil {
		return time.Time{}, err
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return time.Time{}, err
	}

	return commit.Author.When, nil
}

// Cache wraps the GetLatestCommitDate method, caching, and on a schedule
// re-retrieving, requested repo information.
type Cache struct {
	stop func()

	mu    sync.RWMutex
	cache map[string]time.Time
}

// NewCache creates a cache storing the last commit date for git repos, that
// will re-retrieve commit information on a timeout specified by the given
// Duration.
//
// The Stop() method must be before replacing (or otherwise losing this pointer
// to) this cache.
func NewCache(d time.Duration) *Cache {
	ctx, fn := context.WithCancel(context.Background())

	cache := &Cache{
		cache: make(map[string]time.Time),
		stop:  fn,
	}

	if d > 0 {
		go cache.runCache(ctx, d)
	}

	return cache
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
		c.mu.RUnlock()

		updates := make(map[string]time.Time)

		for _, repo := range keys {
			commitDate, err := GetLatestCommitDate(repo)
			if err != nil {
				continue
			}

			updates[repo] = commitDate
		}

		c.mu.Lock()
		maps.Copy(c.cache, updates)
		c.mu.Unlock()
	}
}

// GetBackupActivity retrieves a cache using the given path, and then calls the
// normal GetBackupActivity method.
func (c *Cache) GetLatestCommitDate(repo string) (time.Time, error) {
	c.mu.RLock()
	existing, ok := c.cache[repo]
	c.mu.RUnlock()

	if ok {
		return existing, nil
	}

	t, err := GetLatestCommitDate(repo)

	c.mu.Lock()
	c.cache[repo] = t
	c.mu.Unlock()

	return t, err
}

// Stop stops the concurrent retrieval of backup statuses.
func (c *Cache) Stop() {
	c.stop()
}
