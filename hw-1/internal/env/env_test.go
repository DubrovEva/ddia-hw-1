package env

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	t.Setenv("ENV_TEST", "")
	assert.Equal(t, "def", String("ENV_TEST", "def"))

	t.Setenv("ENV_TEST", "val")
	assert.Equal(t, "val", String("ENV_TEST", "def"))
}

func TestRequired(t *testing.T) {
	t.Setenv("ENV_TEST", "")
	_, err := Required("ENV_TEST")
	require.Error(t, err)

	t.Setenv("ENV_TEST", "val")
	v, err := Required("ENV_TEST")
	require.NoError(t, err)
	assert.Equal(t, "val", v)
}

func TestDuration(t *testing.T) {
	t.Setenv("ENV_TEST", "")
	d, err := Duration("ENV_TEST", time.Second)
	require.NoError(t, err)
	assert.Equal(t, time.Second, d)

	t.Setenv("ENV_TEST", "2m")
	d, err = Duration("ENV_TEST", time.Second)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, d)

	t.Setenv("ENV_TEST", "nope")
	_, err = Duration("ENV_TEST", time.Second)
	assert.Error(t, err)
}

func TestSecretFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	require.NoError(t, os.WriteFile(path, []byte("  s3cret\n"), 0o600))

	t.Setenv("ENV_TEST", path)
	v, err := SecretFile("ENV_TEST")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", v)

	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(empty, []byte("\n"), 0o600))
	t.Setenv("ENV_TEST", empty)
	_, err = SecretFile("ENV_TEST")
	require.Error(t, err)

	t.Setenv("ENV_TEST", filepath.Join(dir, "missing"))
	_, err = SecretFile("ENV_TEST")
	require.Error(t, err)
}

func TestFloat(t *testing.T) {
	t.Setenv("ENV_TEST", "")
	f, err := Float("ENV_TEST", 0.5)
	require.NoError(t, err)
	assert.InDelta(t, 0.5, f, 1e-9)

	t.Setenv("ENV_TEST", "0.05")
	f, err = Float("ENV_TEST", 0.5)
	require.NoError(t, err)
	assert.InDelta(t, 0.05, f, 1e-9)

	t.Setenv("ENV_TEST", "x")
	_, err = Float("ENV_TEST", 0.5)
	assert.Error(t, err)
}

func TestInt(t *testing.T) {
	t.Setenv("ENV_TEST", "")
	n, err := Int("ENV_TEST", 3)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	t.Setenv("ENV_TEST", "42")
	n, err = Int("ENV_TEST", 3)
	require.NoError(t, err)
	assert.Equal(t, 42, n)

	t.Setenv("ENV_TEST", "4.2")
	_, err = Int("ENV_TEST", 3)
	assert.Error(t, err)
}

func TestList(t *testing.T) {
	t.Setenv("ENV_TEST", "")
	assert.Equal(t, []string{"d"}, List("ENV_TEST", []string{"d"}))

	t.Setenv("ENV_TEST", " a, b ,,c ")
	assert.Equal(t, []string{"a", "b", "c"}, List("ENV_TEST", nil))
}
