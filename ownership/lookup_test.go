package ownership

import (
	"os/user"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOsLookup(t *testing.T) {
	lookup := Os{}

	current, err := user.Current()
	require.NoError(t, err)

	uid, err := lookup.UID(current.Username)
	require.NoError(t, err)

	assert.Equal(t, current.Uid, strconv.Itoa(int(uid)))

	currentgids, err := current.GroupIds()
	require.NoError(t, err)

	for _, gid := range currentgids {
		group, err := user.LookupGroupId(gid)
		assert.NoError(t, err)
		lookedupGid, err := lookup.GID(group.Name)
		assert.NoError(t, err)

		assert.Equal(t, gid, strconv.Itoa(int(lookedupGid)))
	}
}
