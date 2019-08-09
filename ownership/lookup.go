package ownership

import (
	"fmt"
	"os/user"
	"strconv"
)

// Lookup is the interface needed by Keysync to resolve a user ID from their username (and group)
// It is intended to be used with the implementation on Os.  There's also one in mock.go that
// uses fixed data instead of operating-system sourced data.
type Lookup interface {
	UID(username string) (uint32, error)
	GID(groupname string) (uint32, error)
}

// Os implements Lookup using the os/user standard library package
type Os struct{}

var _ Lookup = Os{}

func (o Os) UID(username string) (uint32, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return 0, fmt.Errorf("Error resolving uid for %s: %v\n", username, err)
	}
	id, err := strconv.ParseUint(u.Uid, 10 /* base */, 32 /* bits */)
	if err != nil {
		return 0, fmt.Errorf("Error parsing uid %s for %s: %v\n", u.Uid, username, err)
	}
	return uint32(id), nil
}

func (o Os) GID(groupname string) (uint32, error) {
	group, err := user.LookupGroup(groupname)
	if err != nil {
		return 0, fmt.Errorf("Error resolving gid for %s: %v\n", group, err)
	}
	id, err := strconv.ParseUint(group.Gid, 10 /* base */, 32 /* bits */)
	if err != nil {
		return 0, fmt.Errorf("Error parsing gid %s for %s: %v\n", group.Gid, groupname, err)
	}
	return uint32(id), nil
}
