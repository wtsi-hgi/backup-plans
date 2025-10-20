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
	"time"
)

type groups struct {
	expiry time.Time
	uid    uint32
	groups []uint32
}

var userGroupsCache = makeMuMap[string, groups]() //nolint:gochecknoglobals

// GetIDs returns the UID and a slice of GIDs for the given username.
//
// Returns 0, nil when the user cannot be found.
func GetIDs(username string) (uint32, []uint32) {
	if gc, ok := userGroupsCache.Get(username); ok && !gc.expiry.After(time.Now()) {
		return gc.uid, gc.groups
	}

	uid, gids := getUserData(username)
	if gids == nil {
		return 0, nil
	}

	userGroupsCache.Set(username, groups{
		expiry: time.Now().Add(time.Hour),
		uid:    uid,
		groups: gids,
	})

	return uid, gids
}

func getUserData(username string) (uint32, []uint32) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, nil
	}

	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return 0, nil
	}

	gids, err := u.GroupIds()
	if err != nil {
		return 0, nil
	}

	gs := make([]uint32, 0, len(gids))

	for _, gid := range gids {
		g, err := strconv.ParseUint(gid, 10, 32)
		if err != nil {
			return 0, nil
		}

		gs = append(gs, uint32(g))
	}

	return uint32(uid), gs
}
