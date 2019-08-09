package ownership

import "fmt"

// Mock implements the lookup interface using a fixed set of users and groups, useful for tests
type Mock struct {
	Users  map[string]uint32
	Groups map[string]uint32
}

var _ Lookup = &Mock{}

func (m *Mock) UID(username string) (uint32, error) {
	uid, ok := m.Users[username]
	if !ok {
		return 0, fmt.Errorf("unknown user %s", username)
	}
	return uid, nil
}

func (m *Mock) GID(username string) (uint32, error) {
	uid, ok := m.Groups[username]
	if !ok {
		return 0, fmt.Errorf("unknown group %s", username)
	}
	return uid, nil
}
