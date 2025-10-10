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

var userGroupsCache = makeMuMap[string, groups]()

// GetIDs returns the UID and a slice of GIDs for the given username.
//
// Returns 0, nil when the user cannot be found.
func GetIDs(username string) (uint32, []uint32) {
	if gc, ok := userGroupsCache.Get(username); ok && gc.expiry.After(time.Now()) {
		return gc.uid, gc.groups
	}

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

	userGroupsCache.Set(username, groups{
		expiry: time.Now().Add(time.Hour),
		uid:    uint32(uid),
		groups: gs,
	})

	return uint32(uid), gs
}
