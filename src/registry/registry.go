// Package registry resolves artifacts in data registries: Dud projects kept
// under git whose committed artifacts live on an rclone remote. It shells out
// to git (as the cache does to rclone) and never keeps a clone around.
package registry

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kevin-hanselman/dud/src/agglog"
	"github.com/kevin-hanselman/dud/src/artifact"
	"github.com/kevin-hanselman/dud/src/stage"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// runGit runs git in dir and returns its stdout. It is a variable so tests
// can swap in a fake.
var runGit = func(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

// A Lookup is the result of resolving an artifact in a registry.
type Lookup struct {
	// RevLock is the commit SHA the requested revision resolved to.
	RevLock string
	// Remote is the rclone remote path holding the artifact.
	Remote string
	// StagePath is the registry stage that owns the artifact.
	StagePath string
	// Artifact is a copy of the owning stage's output.
	Artifact artifact.Artifact
}

// A Registry is a scratch git repository used to fetch registry revisions
// into. Close removes it.
type Registry struct {
	tmpDir string
	// resolved memoizes revision lookups per repo and rev so that updating
	// many stages from one registry costs a single fetch.
	resolved map[string]string
	logger   *agglog.AggLogger
}

// New creates the scratch repository.
func New(logger *agglog.AggLogger) (*Registry, error) {
	tmpDir, err := os.MkdirTemp("", "dud-registry-")
	if err != nil {
		return nil, err
	}
	if _, err := runGit(tmpDir, "init", "-q"); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	return &Registry{
		tmpDir:   tmpDir,
		resolved: make(map[string]string),
		logger:   logger,
	}, nil
}

// Close removes the scratch repository.
func (reg *Registry) Close() error {
	return os.RemoveAll(reg.tmpDir)
}

// Resolve looks up artPath in the registry at repo/rev. An empty rev means
// the registry's default branch.
func (reg *Registry) Resolve(repo, rev, artPath string) (Lookup, error) {
	var lookup Lookup
	sha, err := reg.resolveRev(repo, rev)
	if err != nil {
		return lookup, err
	}
	lookup.RevLock = sha

	stagePaths, err := reg.stagePaths(repo, sha)
	if err != nil {
		return lookup, err
	}

	var owner *stage.Stage
	for _, stagePath := range stagePaths {
		stg, err := reg.loadStage(sha, stagePath)
		if err != nil {
			return lookup, err
		}
		if art, ok := stg.Outputs[artPath]; ok {
			owner = &stg
			lookup.StagePath = stagePath
			lookup.Artifact = *art
			break
		}
		if parent, ok := stage.FindDirArtifactOwnerForPath(artPath, stg.Outputs); ok {
			return lookup, fmt.Errorf(
				"%s is inside directory artifact %s (stage %s); import %s instead",
				artPath,
				parent.Path,
				stagePath,
				parent.Path,
			)
		}
	}
	if owner == nil {
		return lookup, fmt.Errorf("no stage in %s owns %s", repo, artPath)
	}
	if lookup.Artifact.Checksum == "" {
		return lookup, fmt.Errorf(
			"%s is not committed in %s (stage %s has no checksum for it)",
			artPath,
			repo,
			lookup.StagePath,
		)
	}
	if lookup.Artifact.SkipCache {
		return lookup, fmt.Errorf(
			"%s is not cached in %s (stage %s marks it skip-cache)",
			artPath,
			repo,
			lookup.StagePath,
		)
	}

	// A registry stage may itself be an import, in which case the data lives
	// on the upstream registry's remote (push skips imports).
	if owner.IsImport() {
		if owner.Import.Remote == "" {
			return lookup, fmt.Errorf(
				"%s is imported by %s in %s but has no remote",
				artPath,
				lookup.StagePath,
				repo,
			)
		}
		lookup.Remote = owner.Import.Remote
		return lookup, nil
	}
	lookup.Remote, err = reg.remote(repo, sha)
	return lookup, err
}

// resolveRev fetches rev from repo and returns the commit it points to.
func (reg *Registry) resolveRev(repo, rev string) (string, error) {
	key := repo + "\x00" + rev
	if sha, ok := reg.resolved[key]; ok {
		return sha, nil
	}
	ref := rev
	if ref == "" {
		ref = "HEAD"
	}
	// git runs inside the scratch repository, so a local registry path
	// (relative to the current directory) has to be made absolute.
	target := repo
	if IsLocalPath(repo) {
		var err error
		if target, err = filepath.Abs(repo); err != nil {
			return "", err
		}
	}
	reg.logger.Debug.Printf("fetching %s from %s\n", ref, repo)
	// Protocol v2 lets a full commit SHA be fetched directly on hosts that
	// allow it (e.g. GitHub); a shallow fetch keeps the cost to one commit.
	if _, err := runGit(
		reg.tmpDir,
		"-c", "protocol.version=2",
		"fetch", "-q", "--depth", "1",
		target, ref,
	); err != nil {
		return "", errors.Wrapf(
			err,
			"fetch revision %s from %s (use a tag, a branch or a full commit SHA)",
			ref,
			repo,
		)
	}
	// ^{commit} peels annotated tags to the commit they point to.
	out, err := runGit(reg.tmpDir, "rev-parse", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(out))
	reg.resolved[key] = sha
	return sha, nil
}

func (reg *Registry) show(sha, path string) ([]byte, error) {
	return runGit(reg.tmpDir, "show", sha+":"+path)
}

func (reg *Registry) stagePaths(repo, sha string) ([]string, error) {
	out, err := reg.show(sha, ".dud/index")
	if err != nil {
		return nil, fmt.Errorf("%s is not a Dud project at the requested revision (no .dud/index)", repo)
	}
	var paths []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, scanner.Err()
}

func (reg *Registry) loadStage(sha, stagePath string) (stage.Stage, error) {
	out, err := reg.show(sha, stagePath)
	if err != nil {
		return stage.Stage{}, err
	}
	return stage.FromReader(bytes.NewReader(out), stagePath)
}

// config mirrors the remote-related keys of a Dud project config file. The
// registry's config is read on its own, never merged into this process's
// viper config.
type config struct {
	Remote  string            `yaml:"remote"`
	Remotes map[string]string `yaml:"remotes"`
}

// remote returns the registry's default rclone remote path. The rules
// mirror cmd.resolveRemote: `remote` names an entry in `remotes`, or is
// itself an rclone path. (The cmd version is bound to viper, hence the
// duplication.)
func (reg *Registry) remote(repo, sha string) (string, error) {
	var cfg config
	// A missing config file is the same as an empty one.
	if out, err := reg.show(sha, ".dud/config.yaml"); err == nil {
		if err := yaml.Unmarshal(out, &cfg); err != nil {
			return "", errors.Wrapf(err, "parse .dud/config.yaml in %s", repo)
		}
	}
	if cfg.Remote == "" {
		return "", fmt.Errorf("%s declares no remote in .dud/config.yaml", repo)
	}
	// Viper lower-cases map keys, so the registry's own tooling would have
	// matched names case-insensitively.
	for name, path := range cfg.Remotes {
		if strings.EqualFold(name, cfg.Remote) {
			return path, nil
		}
	}
	return cfg.Remote, nil
}

// scpLikeURL matches git's "user@host:path" syntax: a colon before any
// slash.
var scpLikeURL = regexp.MustCompile(`^[^/]+:`)

// IsLocalPath reports whether repo is a filesystem path rather than a URL.
// Local paths must be made absolute before Dud changes into the project
// root.
func IsLocalPath(repo string) bool {
	if strings.Contains(repo, "://") {
		return false
	}
	return !scpLikeURL.MatchString(repo)
}
