package users

import (
	"os/user"
	"strconv"
)

var (
	userCache  = make(map[uint32]string)
	groupCache = make(map[uint32]string)
)

func Username(uid uint32) string {
	if u, ok := userCache[uid]; ok {
		return u
	}

	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return ""
	}

	userCache[uid] = u.Username

	return u.Username
}

func Group(gid uint32) string {
	if g, ok := groupCache[gid]; ok {
		return g
	}

	g, err := user.LookupGroupId(strconv.FormatUint(uint64(gid), 10))
	if err != nil {
		return ""
	}

	userCache[gid] = g.Name

	return g.Name
}
