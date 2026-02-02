/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
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

package git

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/protocol/packp"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/storage/memory"
)

// GetLatestCommitDate returns the commit date for the HEAD of the supplied
// repo.
func GetLatestCommitDate(url string) (time.Time, error) {
	repo, err := getRepo(url)
	if err != nil {
		return time.Time{}, err
	}

	refs, err := repo.References()
	if err != nil {
		return time.Time{}, err
	}

	var latestCommitDate time.Time

	err = refs.ForEach(func(r *plumbing.Reference) error {
		if r.Type() == plumbing.SymbolicReference {
			return nil
		}

		commit, errr := repo.CommitObject(r.Hash())
		if errr != nil {
			return errr
		}

		if commit.Author.When.After(latestCommitDate) {
			latestCommitDate = commit.Author.When
		}

		return nil
	})

	return latestCommitDate, err
}

func getRepo(url string) (*git.Repository, error) {
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
		return nil, err
	}

	return repo, nil
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
