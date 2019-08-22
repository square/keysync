package backup

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/square/keysync/output"

	"github.com/pkg/errors"
)

// Given a path to a directory, create and return a tarball of its content.
// Careful, as this will pull the full contents into memory.
// This is not a general-purpose function, but is intended to only work with Keysync
// directories, which contain only non-executable regular files.
func createTar(dir string) ([]byte, error) {
	var tarball bytes.Buffer
	tw := tar.NewWriter(&tarball)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !info.Mode().IsRegular() {
			// Skip directories and non-regular files.
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close() // We explicitly call close below with error handling, but this extra one handles early returns

		// 2nd Argument to FileInfoHeader is only used for symlinks, which aren't relevant here
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Set the name to be relative to the base directory
		header.Name, err = filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if _, err := io.Copy(tw, f); err != nil {
			return err
		}

		if err := f.Close(); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Tar writing adds a trailer in Close(), and can possibly return errors, so we need to check
	// errors here.  We could also defer tw.Close(), but there's nothing to leak in the Tar Writer
	// other than the io.Writer that's passed in.  That's the tarball buffer in this function, so
	// we don't need to worry about leaking FDs. Calling Close() a 2nd time is always an error, so
	// I think it makes the error handling trickier if we both explicitly and defer a call to Close.
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return tarball.Bytes(), nil
}

// Given a tarball, write it out to dir, which must be empty or not exist
// If Chown is true, set file ownership from the tarball.
// This is intended to be only used with files from createTar.
func extractTar(tarball []byte, chown bool, dirpath string, filesystem output.Filesystem) error {
	// Open the destination directory and verify it's empty
	dir, err := os.Open(dirpath)
	if os.IsNotExist(err) {
		// The directory doesn't exist, so try to make it.
		if err := os.MkdirAll(dirpath, 0755); err != nil {
			return errors.Wrapf(err, "could not create secrets directory %s", dirpath)
		}
		dir, err = os.Open(dirpath)
	}
	if err != nil {
		return errors.Wrapf(err, "error opening secrets directory %s", dirpath)
	}

	info, err := dir.Stat()
	if err != nil {
		return errors.Wrapf(err, "could not stat directory %s", dirpath)
	}
	if !info.IsDir() {
		return fmt.Errorf("secrets directory exists but is a file %s", dirpath)
	}

	// Check if the directory is empty
	if list, err := dir.Readdir(-1); err != nil {
		return errors.Wrapf(err, "could not read contents of secrets directory %s", dirpath)
	} else if len(list) != 0 {
		return fmt.Errorf("secrets directory exists but is nonempty (%d entries)", len(list))
	}

	// At this point, the directory exists and is non-empty, so let's unpack files there
	tr := tar.NewReader(bytes.NewReader(tarball))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "error reading tar header")
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// We don't need to care about directories, because they're created by WriteFileAtomically
		case tar.TypeReg:
			fileInfo := output.FileInfo{Mode: os.FileMode(header.Mode), UID: header.Uid, GID: header.Gid}

			content, err := ioutil.ReadAll(tr)
			if err != nil {
				return errors.Wrapf(err, "error reading %s", header.Name)
			}

			if header.Name != filepath.Clean(header.Name) {
				return fmt.Errorf("non-canonical file path in archive: %s", header.Name)
			}
			path := filepath.Join(dirpath, header.Name)

			if _, err := output.WriteFileAtomically(path, chown, fileInfo, filesystem, content); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unhandled file %s of type %v", header.Name, header.Typeflag)
		}
	}

	return nil
}
