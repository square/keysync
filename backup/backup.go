// package backup handles reading and writing encrypted .tar files from the secretsDirectory to
// a backupPath using the key backupKey.
package backup

import (
	"encoding/hex"
	"io/ioutil"

	"github.com/square/keysync/output"

	"github.com/pkg/errors"
)

type Backup interface {
	Backup() error
	Restore() error
}

type FileBackup struct {
	SecretsDirectory string
	BackupPath       string
	KeyPath          string
	Chown            bool
	EnforceFS        output.Filesystem
}

// Backup is intended to be implemented by FileBackup
var _ Backup = &FileBackup{}

func (b *FileBackup) loadKey() ([]byte, error) {
	keyhex, err := ioutil.ReadFile(b.KeyPath)
	if err != nil {
		return nil, err
	}
	key := make([]byte, hex.DecodedLen(len(keyhex)))
	if _, err := hex.Decode(key, keyhex); err != nil {
		return nil, err
	}
	return key, nil
}

// Backup loads all files in b.SecretsDirectory, tars, compresses, then encrypts with b.BackupKey
// The content is written to b.BackupPath
func (b *FileBackup) Backup() error {
	tarball, err := createTar(b.SecretsDirectory)
	if err != nil {
		return err
	}

	key, err := b.loadKey()
	if err != nil {
		return err
	}

	// Encrypt it
	encrypted, err := encrypt(tarball, key)
	if err != nil {
		return errors.Wrap(err, "error encrypting backup")
	}

	// We always write as r-- --- ---, aka 0400
	// UID/GID in this struct are ignored as chownFiles: false
	perms := output.FileInfo{Mode: 0400}
	// Write it out, and if it errored, wrapped the error
	_, err = output.WriteFileAtomically(b.BackupPath, false, perms, 0, encrypted)
	return err
}

// Restore opens b.BackupPath, decrypts with b.BackupKey, and writes contents to b.SecretsDirectory
func (b *FileBackup) Restore() error {
	ciphertext, err := ioutil.ReadFile(b.BackupPath)
	if err != nil {
		return errors.Wrap(err, "error reading backup")
	}

	key, err := b.loadKey()
	if err != nil {
		return err
	}

	tarball, err := decrypt(ciphertext, key)
	if err != nil {
		return errors.Wrap(err, "error decrypting backup")
	}

	if err := extractTar(tarball, b.Chown, b.SecretsDirectory, b.EnforceFS); err != nil {
		return errors.Wrap(err, "Error extracting tarball")
	}

	return nil
}
