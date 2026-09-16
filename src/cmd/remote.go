package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// remoteName is the value of the --remote/-r flag shared by push, fetch and
// pull.
var remoteName string

type noRemoteError struct{}

func (e noRemoteError) Error() string {
	return "no remote specified in the config"
}

type unknownRemoteError struct {
	name string
}

func (e unknownRemoteError) Error() string {
	return fmt.Sprintf(
		"unknown remote '%s'; add it under 'remotes' in the config",
		e.name,
	)
}

// configuredRemotes returns the `remotes` config map. Viper lower-cases map
// keys, so lookups must be done on the lower-cased name.
func configuredRemotes() map[string]string {
	return viper.GetStringMapString("remotes")
}

// resolveRemote turns a remote name into the rclone remote path handed to
// the cache. An empty name means "use the default from the config".
//
// The config's `remote` key can hold either a name from the `remotes` map or
// (as in projects created before named remotes existed) a literal rclone
// path. A name that is passed explicitly, however, must exist in `remotes`,
// otherwise a typo would silently be sent to rclone as a path.
func resolveRemote(name string) (string, error) {
	explicit := name != ""
	if !explicit {
		name = viper.GetString("remote")
		if name == "" {
			return "", noRemoteError{}
		}
	}
	if path, ok := configuredRemotes()[strings.ToLower(name)]; ok {
		return path, nil
	}
	if explicit {
		return "", unknownRemoteError{name}
	}
	return name, nil
}

// remoteFromArgs decides which remote push/fetch/pull should use and strips a
// leading remote name from the stage paths.
//
// It must be called after prepare(), which both loads the config and rewrites
// the CLI args in place so they are relative to the project root. That
// rewrite is why rawArgs (a copy taken before prepare()) is needed: from a
// sub-directory, a remote name "foo" would already have become "sub/foo" in
// paths. If the --remote flag is set, positional detection is skipped so the
// first arg is always a stage path.
func remoteFromArgs(rawArgs, paths []string) (string, []string, error) {
	name := remoteName
	if name == "" && len(rawArgs) > 0 {
		if _, ok := configuredRemotes()[strings.ToLower(rawArgs[0])]; ok {
			name = rawArgs[0]
			paths = paths[1:]
		}
	}
	remote, err := resolveRemote(name)
	if err != nil {
		return "", nil, err
	}
	return remote, paths, nil
}
