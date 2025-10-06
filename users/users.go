package users

import (
	"os/user"
	"strconv"
	"sync"
)

type muMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func (m *muMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.m[key]

	return v, ok
}

func (m *muMap[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.m[key] = value
}

func makeMuMap[K comparable, V any]() *muMap[K, V] {
	return &muMap[K, V]{
		m: make(map[K]V),
	}
}

var (
	userCache  = makeMuMap[uint32, string]()
	groupCache = makeMuMap[uint32, string]()
)

func Username(uid uint32) string {
	if u, ok := userCache.Get(uid); ok {
		return u
	}

	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return ""
	}

	userCache.Set(uid, u.Username)

	return u.Username
}

func Group(gid uint32) string {
	if g, ok := groupCache.Get(gid); ok {
		return g
	}

	g, err := user.LookupGroupId(strconv.FormatUint(uint64(gid), 10))
	if err != nil {
		return ""
	}

	groupCache.Set(gid, g.Name)

	return g.Name
}
