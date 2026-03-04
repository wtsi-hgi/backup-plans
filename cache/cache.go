package cache

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"
)

// Cache wraps a function, caching, and on a schedule re-retrieving, requested
// information.
type Cache[K comparable, T any] struct {
	stop  func()
	getFn func(K) (T, error)

	mu    sync.RWMutex
	cache map[K]T
}

// New creates a cache storing the last response from a function, given a key.
//
// The information will be re-retrieve on a timeout specified by the given
// Duration.
//
// The Stop() method must be before replacing (or otherwise losing this pointer
// to) this cache.
func New[K comparable, V any](d time.Duration, getFn func(K) (V, error)) *Cache[K, V] {
	ctx, fn := context.WithCancel(context.Background())

	cache := &Cache[K, V]{
		cache: make(map[K]V),
		stop:  fn,
		getFn: getFn,
	}

	if d > 0 {
		go cache.runCache(ctx, d)
	}

	return cache
}

func (c *Cache[K, V]) runCache(ctx context.Context, d time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}

		c.mu.RLock()
		keys := slices.Collect(maps.Keys(c.cache))
		c.mu.RUnlock()

		updates := make(map[K]V)

		for _, key := range keys {
			v, err := c.getFn(key)
			if err != nil {
				continue
			}

			updates[key] = v
		}

		c.mu.Lock()
		maps.Copy(c.cache, updates)
		c.mu.Unlock()
	}
}

// Get attempts to retrieve a cached item using the given key.
//
// If the item doesn't exist, it will call the function given to 'New' to get an
// item, then cache it and return it.
func (c *Cache[K, T]) Get(key K) (T, error) {
	c.mu.RLock()
	existing, ok := c.cache[key]
	c.mu.RUnlock()

	if ok {
		return existing, nil
	}

	t, err := c.getFn(key)

	c.mu.Lock()
	c.cache[key] = t
	c.mu.Unlock()

	return t, err
}

// Stop stops the re-generating of cached items after a timeout.
func (c *Cache[K, T]) Stop() {
	c.stop()
}
