package cmd

import (
	"errors"
	"testing"

	"github.com/kevin-hanselman/dud/src/index"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetRemoteConfig(t *testing.T) {
	t.Helper()
	viper.Reset()
	remoteName = ""
	t.Cleanup(func() {
		viper.Reset()
		remoteName = ""
	})
}

func TestResolveRemote(t *testing.T) {
	t.Run("no config returns noRemoteError", func(t *testing.T) {
		resetRemoteConfig(t)

		_, err := resolveRemote("")
		assert.True(t, errors.Is(err, index.NoRemoteError{}))
	})

	t.Run("legacy literal remote is returned verbatim", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remote", "fake_remote:/tmp/cache")

		got, err := resolveRemote("")
		require.NoError(t, err)
		assert.Equal(t, "fake_remote:/tmp/cache", got)
	})

	t.Run("default name resolves through remotes map", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo", "bar": "s3:bar"})
		viper.Set("remote", "foo")

		got, err := resolveRemote("")
		require.NoError(t, err)
		assert.Equal(t, "s3:foo", got)
	})

	t.Run("explicit name resolves through remotes map", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo", "bar": "s3:bar"})
		viper.Set("remote", "foo")

		got, err := resolveRemote("bar")
		require.NoError(t, err)
		assert.Equal(t, "s3:bar", got)
	})

	t.Run("explicit unknown name is an error", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo"})

		_, err := resolveRemote("nope")
		assert.True(t, errors.Is(err, unknownRemoteError{"nope"}))
	})

	t.Run("lookup is case-insensitive", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo"})

		got, err := resolveRemote("FOO")
		require.NoError(t, err)
		assert.Equal(t, "s3:foo", got)
	})
}

func TestRemoteFromArgs(t *testing.T) {
	t.Run("leading remote name is stripped", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo", "bar": "s3:bar"})
		viper.Set("remote", "foo")

		remote, paths, err := remoteFromArgs(
			[]string{"bar", "a.yaml"},
			[]string{"sub/bar", "sub/a.yaml"},
		)
		require.NoError(t, err)
		assert.Equal(t, "s3:bar", remote)
		assert.Equal(t, []string{"sub/a.yaml"}, paths)
	})

	t.Run("non-remote first arg is kept as a stage path", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo"})
		viper.Set("remote", "foo")

		remote, paths, err := remoteFromArgs(
			[]string{"a.yaml", "b.yaml"},
			[]string{"a.yaml", "b.yaml"},
		)
		require.NoError(t, err)
		assert.Equal(t, "s3:foo", remote)
		assert.Equal(t, []string{"a.yaml", "b.yaml"}, paths)
	})

	t.Run("flag skips positional detection", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo", "bar": "s3:bar"})
		remoteName = "bar"

		remote, paths, err := remoteFromArgs(
			[]string{"foo"},
			[]string{"foo"},
		)
		require.NoError(t, err)
		assert.Equal(t, "s3:bar", remote)
		assert.Equal(t, []string{"foo"}, paths)
	})

	t.Run("no args uses default remote", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remotes", map[string]string{"foo": "s3:foo"})
		viper.Set("remote", "foo")

		remote, paths, err := remoteFromArgs(nil, nil)
		require.NoError(t, err)
		assert.Equal(t, "s3:foo", remote)
		assert.Empty(t, paths)
	})

	t.Run("no remotes configured falls back to legacy remote", func(t *testing.T) {
		resetRemoteConfig(t)
		viper.Set("remote", "fake_remote")

		remote, paths, err := remoteFromArgs(
			[]string{"a.yaml"},
			[]string{"a.yaml"},
		)
		require.NoError(t, err)
		assert.Equal(t, "fake_remote", remote)
		assert.Equal(t, []string{"a.yaml"}, paths)
	})
}

func TestValidateConfigField(t *testing.T) {
	assert.NoError(t, validateConfigField("remote", false))
	assert.NoError(t, validateConfigField("remotes.foo", false))
	assert.NoError(t, validateConfigField("remotes", true))
	assert.Error(t, validateConfigField("remotes", false))
	assert.Error(t, validateConfigField("remotes.", false))
	assert.Error(t, validateConfigField("remotes.a.b", false))
	assert.Error(t, validateConfigField("bogus", true))
}
