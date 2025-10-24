/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
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

func (m *muMap[K, V]) Get(key K) (V, bool) { //nolint:ireturn,nolintlint
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
	userCache  = makeMuMap[uint32, string]() //nolint:gochecknoglobals
	groupCache = makeMuMap[uint32, string]() //nolint:gochecknoglobals
)

// Username returns the username assigned to the given UID.
//
// Returns empty string if the UID doesn't exist.
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

// Group returns the group name assigned to the given GID.
//
// Returns empty string if the GID doesn't exist.
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
