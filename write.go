package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// atomicWrite creates a temporary file, sets perms, writes content, and renames it to filename
// This sequence ensures the following:
// 1. Nobody can open the file before we set owner/permissions properly
// 2. Nobody observes a partially-overwritten secret file.
func atomicWrite(name string, secret *Secret, defaultOwner Ownership) error {
	// We can't use ioutil.TempFile because we want to open 0000.
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return err
	}
	rand := hex.EncodeToString(buf)
	f, err := os.OpenFile(name+rand, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0000)
	if err != nil {
		return err
	}
	ownership := secret.OwnershipValue(defaultOwner)
	err = f.Chown(int(ownership.Uid), int(ownership.Gid))
	if err != nil {
		// TODO: We will fail as non-root/CAP_CHOWN. Bad in prod, but don't want to test as root.
		fmt.Printf("Chown failed: %v\n", err)
	}
	// Always Chmod after the Chown, so we don't expose secret with the wrong owner.
	err = f.Chmod(os.FileMode(secret.ModeValue()))
	if err != nil {
		return err
	}

	f.Write(secret.Content)

	// Rename is atomic, so nobody will observe a partially updated secret
	err = os.Rename(name+rand, name)
	return err
}
