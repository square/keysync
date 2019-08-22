package output

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FileInfo returns the filesystem properties atomicWrite wrote
type FileInfo struct {
	Mode os.FileMode
	UID  int
	GID  int
}

// GetFileInfo from an open file
func GetFileInfo(file *os.File) (*FileInfo, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat after writing: %v", err)
	}
	filemode := stat.Mode()
	uid := int(stat.Sys().(*syscall.Stat_t).Uid)
	gid := int(stat.Sys().(*syscall.Stat_t).Gid)

	return &FileInfo{filemode, uid, gid}, nil
}

// WriteFileAtomically creates a temporary file, sets perms, writes content, and renames it to filename
// This sequence ensures the following:
// 1. Nobody can open the file before we set owner/permissions properly
// 2. Nobody observes a partially-overwritten secret file.
// The returned FileInfo may not match the passed in one, especially if chownFiles is false.
func WriteFileAtomically(dir, filename string, chownFiles bool, fileInfo FileInfo, enforceFilesystem Filesystem, content []byte) (*FileInfo, error) {
	if err := os.MkdirAll(dir, 0775); err != nil {
		return nil, fmt.Errorf("making client directory '%s': %v", dir, err)
	}

	// We can't use ioutil.TempFile because we want to open 0000.
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		return nil, err
	}
	randSuffix := hex.EncodeToString(buf)
	fullPath := filepath.Join(dir, filename)
	f, err := os.OpenFile(fullPath+randSuffix, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0000)
	// Try to remove the file, in event we early-return with an error.
	defer os.Remove(fullPath + randSuffix)
	if err != nil {
		return nil, err
	}

	if chownFiles {
		err = f.Chown(fileInfo.UID, fileInfo.GID)
		if err != nil {
			return nil, err
		}
	}

	// Always Chmod after the Chown, so we don't expose secret with the wrong owner.
	err = f.Chmod(fileInfo.Mode)
	if err != nil {
		return nil, err
	}

	if enforceFilesystem != 0 {
		good, err := isFilesystem(f, enforceFilesystem)
		if err != nil {
			return nil, fmt.Errorf("checking filesystem type: %v", err)
		}
		if !good {
			return nil, fmt.Errorf("unexpected filesystem writing %s", filename)
		}
	}
	_, err = f.Write(content)
	if err != nil {
		return nil, fmt.Errorf("failed writing filesystem content: %v", err)
	}

	fileinfo, err := GetFileInfo(f)
	if err != nil {
		return nil, fmt.Errorf("failed to get file mode back from file: %v", err)
	}

	// While this is intended for use with tmpfs, you could write secrets to disk.
	// We ignore any errors from syncing, as it's not strictly required.
	_ = f.Sync()

	// Rename is atomic, so nobody will observe a partially updated secret
	err = os.Rename(fullPath+randSuffix, fullPath)
	if err != nil {
		return nil, err
	}

	return fileinfo, nil
}

// The Filesystem identification.  On Mac, this is uint32, and int64 on linux
// So both are safe to store as an int64.
// Linux Tmpfs = 0x01021994
// Get these constants with `stat --file-system --format=%t`
type Filesystem int64

func isFilesystem(file *os.File, fs Filesystem) (bool, error) {
	var statfs syscall.Statfs_t
	err := syscall.Fstatfs(int(file.Fd()), &statfs)
	return Filesystem(statfs.Type) == fs, err
}
