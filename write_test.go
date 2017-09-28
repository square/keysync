package keysync

import (
	"io/ioutil"
	"testing"

	"fmt"
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func testConfig(t *testing.T) Config {
	dir, err := ioutil.TempDir("", "keysyncWriteTest")
	assert.NoError(t, err, "Error making tempdir")

	return Config{
		SecretsDir: dir,
		ChownFiles: false,
		PasswdFile: "fixtures/ownership/passwd",
		GroupFile:  "fixtures/ownership/group",
	}
}

func testClientConfig(name string) ClientConfig {
	return ClientConfig{
		Key:     fmt.Sprintf("fixtures/clients/%s.key", name),
		Cert:    fmt.Sprintf("fixtures/clients/%s.crt", name),
		DirName: name,
	}
}

func testSecret(name string) Secret {
	return Secret{
		Name:     name,
		Content:  []byte("my secret content"),
		Checksum: "0ABC",
	}
}

func testFixture(t *testing.T) (Config, ClientConfig, OutputDirCollection, Output) {
	c := testConfig(t)

	testlogger := testLogger()

	odc := OutputDirCollection{Config: &c}
	cc := testClientConfig("client 1")
	out, err := odc.NewOutput(cc, testlogger)
	assert.NoError(t, err)

	return c, cc, odc, out
}

func testLogger() *logrus.Entry {
	return logrus.NewEntry(logrus.New())
}

// Test the basic secret lifecycle:
// Calls all methods in the OutputCollection and Output interface
func TestBasicLifecycle(t *testing.T) {
	c, _, odc, out := testFixture(t)
	defer os.RemoveAll(c.SecretsDir)

	odc.Cleanup(map[string]struct{}{"client 1": {}}, testLogger())

	name := "secret 1"
	s := testSecret(name)

	state, err := out.Write(&s)
	assert.NoError(t, err)

	assert.NoError(t, out.Cleanup(map[string]Secret{name: {}}))

	assert.True(t, out.Validate(&s, *state), "Expected just-written secret to be valid")

	filecontents, err := ioutil.ReadFile(filepath.Join(c.SecretsDir, "client 1", name))
	assert.NoError(t, err)

	assert.Equal(t, s.Content, content(filecontents))

	assert.NoError(t, out.Remove(name))

	assert.False(t, out.Validate(&s, *state), "Expected secret invalid after deletion")

	assert.NoError(t, out.RemoveAll())
}

// Test that if ChownFiles is set, we fail to write out files (since we're not root)
// While this isn't a super-great test, it makes sure ChownFiles = true does something.
func TestChownFiles(t *testing.T) {
	c, _, _, out := testFixture(t)
	defer os.RemoveAll(c.SecretsDir)

	// Easier to modify this after-the-fact so we can share test fixture setup
	out.(*OutputDir).ChownFiles = true

	secret := testSecret("secret")
	_, err := out.Write(&secret)
	assert.Error(t, err, "Expected error writing file.  Maybe you're testing as root?")
}

// This tests enforcing filesystems.  We set an EnforceFilesystem value that won't correspond with any filesystem
// we might run tests on, and thus we should never succeed in writing files.
func TestEnforceFS(t *testing.T) {
	c, _, _, out := testFixture(t)
	defer os.RemoveAll(c.SecretsDir)

	// Easier to modify this after-the-fact so we can share test fixture setup
	// This value is Linux's /proc filesystem, arbitrarily chosen as a filesystem we're not going to be writing to,
	// so that out.Write is guaranteed to fail.
	out.(*OutputDir).EnforceFilesystem = 0x9fa0

	secret := testSecret("secret")
	_, err := out.Write(&secret)
	assert.EqualError(t, err, "Unexpected filesystem writing secret")
}

// Make sure any stray files and directories are cleaned up by Keysync.
func TestCleanup(t *testing.T) {
	c, cc, odc, out := testFixture(t)
	defer os.RemoveAll(c.SecretsDir)

	junkdir := filepath.Join(c.SecretsDir, "junk client")
	assert.NoError(t, os.MkdirAll(junkdir, 0400))

	_, err := os.Stat(junkdir)
	assert.NoError(t, err, "Expected junkdir to exist before cleanup")

	errs := odc.Cleanup(map[string]struct{}{cc.DirName: {}}, testLogger())
	assert.Equal(t, 0, len(errs), "Expected no errors cleaning up")

	_, err = os.Stat(junkdir)
	assert.Error(t, err, "Expected junkdir to be gone after cleanup")

	junkfile := filepath.Join(c.SecretsDir, cc.DirName, "junk file")
	assert.NoError(t, ioutil.WriteFile(junkfile, []byte("my data"), 0400))

	assert.NoError(t, out.Cleanup(map[string]Secret{"secret 1": {}}))

	_, err = os.Stat(junkfile)
	assert.Error(t, err, "Expected file to be gone after cleanup")
}

// TestCustomFilename makes sure we honor the "filename" attribute when writing out files.
func TestCustomFilename(t *testing.T) {
	c, _, _, out := testFixture(t)
	defer os.RemoveAll(c.SecretsDir)

	secret := testSecret("secret_name")
	filename := "override_filename"
	secret.Metadata = map[string]string{
		"filename": filename,
	}

	state, err := out.Write(&secret)
	assert.NoError(t, err)

	assert.NoError(t, out.Cleanup(map[string]Secret{filename: {}}))

	assert.True(t, out.Validate(&secret, *state), "Expected override_filename secret to be valid after cleanup")

	assert.NoError(t, out.Remove(filename))

	assert.False(t, out.Validate(&secret, *state), "Expected secret to be removed")
}
