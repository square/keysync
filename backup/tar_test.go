package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSha(t *testing.T, hash string, file *os.File) {
	sha := sha256.New()
	_, err := io.Copy(sha, file)
	assert.NoError(t, err)
	assert.Equal(t, hash, hex.EncodeToString(sha.Sum(nil)))
}

// TestCreateExtractTar makes a tar of the testdata/ folder, then extracts it to a temp directory
// and validates files and permissions are present as expected.
func TestCreateExtractTar(t *testing.T) {
	var testfiles = []struct {
		path     string
		perm     os.FileMode
		sha2hash string
	}{
		{"alice.txt", 0440, "7708cf9d3d58e7a4e621ec2aa9fd47c678fd4a3411c804df060c041ee6237e4d"},
		{"bob/bob.txt", 0404, "a802f68d223a903e282e310251585f26b1abdfe067854252d0f1bf33d334f768"},
	}

	// Ensure the test files have expected permissions and content beforehand
	for _, test := range testfiles {
		file, err := os.Open(filepath.Join("testdata", test.path))
		require.NoError(t, err)
		testSha(t, test.sha2hash, file)
		require.NoError(t, file.Chmod(test.perm))
	}

	tar, err := createTar("testdata")
	require.NoError(t, err)

	tmpdir, err := ioutil.TempDir("", "test-create-extract-tar")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	err = extractTar(tar, false, tmpdir, 0)
	require.NoError(t, err)

	for _, test := range testfiles {
		file, err := os.Open(filepath.Join(tmpdir, test.path))
		assert.NoError(t, err)
		info, err := file.Stat()
		assert.NoError(t, err)
		assert.EqualValues(t, test.perm, info.Mode().Perm(), "unexpected permissions on %s: %s", test.path, info.Mode().String())

		testSha(t, test.sha2hash, file)
	}
}

// TestCheckIfEmpty makes sure we detect files, which should help us avoid accidentally restoring
// a backup when there's secrets in place.
func TestCheckIfEmpty(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test-create-extract-tar")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	// Case 0: A non-existent path
	assert.Error(t, checkIfEmpty(filepath.Join(tmpdir, "this-path-does-not-exist")))

	// Case 1: An empty directory.
	assert.NoError(t, checkIfEmpty(tmpdir))

	// Case 2: Some directories nested
	nested, err := ioutil.TempDir(tmpdir, "nested")
	require.NoError(t, err)
	assert.NoError(t, checkIfEmpty(tmpdir))

	// Case 3: file in nested.  Should error since there's a file
	myfile := filepath.Join(nested, "myfile")
	assert.NoError(t, ioutil.WriteFile(myfile, []byte("hello world"), 0600))
	assert.Error(t, checkIfEmpty(tmpdir))

	// Case 4: Passed a file instead of a directory
	assert.Error(t, checkIfEmpty(myfile))
}
