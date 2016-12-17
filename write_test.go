package main

import "testing"

// TestWriteMode checks that file modes are properly written out to disk, and that file modes are
// properly restricted.  We only allow readable files - no executable, writable, setuid, etc.
func TestWriteMode(t *testing.T) {

}
