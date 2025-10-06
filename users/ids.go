package users

import (
	"os/user"
	"time"
)

type groups struct {
	expiry time.Time
	groups []string
}

var userGroupsCache = makeMuMap[string, groups]()

func GetGroups(username string) []string {
	if gc, ok := userGroupsCache.Get(username); ok && gc.expiry.After(time.Now()) {
		return gc.groups
	}

	u, err := user.Lookup(username)
	if err != nil {
		return nil
	}

	gids, err := u.GroupIds()
	if err != nil {
		return nil
	}

	gs := make([]string, 0, len(gids))

	for _, gid := range gids {
		g, err := user.LookupGroupId(gid)
		if err != nil {
			return nil
		}

		gs = append(gs, g.Name)
	}

	userGroupsCache.Set(username, groups{
		expiry: time.Now().Add(time.Hour),
		groups: gs,
	})

	return gs
}
