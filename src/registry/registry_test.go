package registry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kevin-hanselman/dud/src/agglog"
	"github.com/kevin-hanselman/dud/src/artifact"
	"github.com/stretchr/testify/require"
)

const (
	sha       = "6c73875a5f5b522f90b5afa9ab12585f64327ca7"
	remoteURL = "git@example.com:org/registry.git"
)

// fakeGit serves a registry from an in-memory map of "<sha>:<path>" to file
// contents and records the fetches it saw.
type fakeGit struct {
	files   map[string]string
	fetches []string
}

func (fake *fakeGit) run(_ string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	switch {
	case joined == "init -q":
		return nil, nil
	case strings.Contains(joined, " fetch "):
		fake.fetches = append(fake.fetches, joined)
		return nil, nil
	case joined == "rev-parse FETCH_HEAD^{commit}":
		return []byte(sha + "\n"), nil
	case args[0] == "show":
		content, ok := fake.files[args[1]]
		if !ok {
			return nil, fmt.Errorf("git %s: fatal: path does not exist", joined)
		}
		return []byte(content), nil
	}
	return nil, fmt.Errorf("unexpected git command: %s", joined)
}

func newFakeRegistry(t *testing.T, files map[string]string) (*Registry, *fakeGit) {
	t.Helper()
	fake := &fakeGit{files: files}
	runGitOrig := runGit
	runGit = fake.run
	t.Cleanup(func() { runGit = runGitOrig })
	reg, err := New(agglog.NewNullLogger())
	require.NoError(t, err)
	t.Cleanup(func() { reg.Close() })
	return reg, fake
}

func registryFiles() map[string]string {
	return map[string]string{
		sha + ":.dud/index": "data.txt.yaml\ndir.yaml\n\nnocache.yaml\nuncommitted.yaml\nchained.yaml\n",
		sha + ":.dud/config.yaml": `remotes:
  s3: s3:registry-bucket
  Local: /tmp/registry
remote: S3
`,
		sha + ":data.txt.yaml": `checksum: abc
outputs:
  data.txt:
    checksum: 7de90e7d
`,
		sha + ":dir.yaml": `outputs:
  data/dir:
    checksum: dircs
    is-dir: true
`,
		sha + ":nocache.yaml": `outputs:
  nocache.bin:
    checksum: xyz
    skip-cache: true
`,
		sha + ":uncommitted.yaml": `outputs:
  uncommitted.bin:
`,
		sha + ":chained.yaml": `import:
  repo: ../upstream
  path: model.bin
  remote: gcs:upstream
outputs:
  model.bin:
    checksum: modelcs
`,
	}
}

func TestResolve(t *testing.T) {
	t.Run("file artifact", func(t *testing.T) {
		reg, fake := newFakeRegistry(t, registryFiles())
		got, err := reg.Resolve(remoteURL, "v1", "data.txt")
		require.NoError(t, err)
		want := Lookup{
			RevLock:   sha,
			Remote:    "s3:registry-bucket",
			StagePath: "data.txt.yaml",
			Artifact:  artifact.Artifact{Path: "data.txt", Checksum: "7de90e7d"},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("Lookup -want +got:\n%s", diff)
		}
		require.Equal(
			t,
			[]string{"-c protocol.version=2 fetch -q --depth 1 " + remoteURL + " v1"},
			fake.fetches,
		)
	})

	t.Run("directory artifact", func(t *testing.T) {
		reg, _ := newFakeRegistry(t, registryFiles())
		got, err := reg.Resolve(remoteURL, "v1", "data/dir")
		require.NoError(t, err)
		require.Equal(t, "dir.yaml", got.StagePath)
		require.True(t, got.Artifact.IsDir)
		require.Equal(t, "dircs", got.Artifact.Checksum)
	})

	t.Run("path inside directory artifact", func(t *testing.T) {
		reg, _ := newFakeRegistry(t, registryFiles())
		_, err := reg.Resolve(remoteURL, "v1", "data/dir/a.txt")
		require.EqualError(
			t,
			err,
			"data/dir/a.txt is inside directory artifact data/dir (stage dir.yaml); import data/dir instead",
		)
	})

	t.Run("no owner", func(t *testing.T) {
		reg, _ := newFakeRegistry(t, registryFiles())
		_, err := reg.Resolve(remoteURL, "v1", "nope.txt")
		require.EqualError(t, err, "no stage in "+remoteURL+" owns nope.txt")
	})

	t.Run("skip-cache artifact", func(t *testing.T) {
		reg, _ := newFakeRegistry(t, registryFiles())
		_, err := reg.Resolve(remoteURL, "v1", "nocache.bin")
		require.ErrorContains(t, err, "marks it skip-cache")
	})

	t.Run("uncommitted artifact", func(t *testing.T) {
		reg, _ := newFakeRegistry(t, registryFiles())
		_, err := reg.Resolve(remoteURL, "v1", "uncommitted.bin")
		require.ErrorContains(t, err, "is not committed in")
	})

	t.Run("chained import uses the upstream remote", func(t *testing.T) {
		files := registryFiles()
		// Even without a remote of its own, the registry can re-export
		// artifacts it imports.
		delete(files, sha+":.dud/config.yaml")
		reg, _ := newFakeRegistry(t, files)
		got, err := reg.Resolve(remoteURL, "v1", "model.bin")
		require.NoError(t, err)
		require.Equal(t, "gcs:upstream", got.Remote)
		require.Equal(t, "modelcs", got.Artifact.Checksum)
	})

	t.Run("not a Dud project", func(t *testing.T) {
		reg, _ := newFakeRegistry(t, map[string]string{})
		_, err := reg.Resolve(remoteURL, "v1", "data.txt")
		require.ErrorContains(t, err, "is not a Dud project")
	})

	t.Run("empty rev fetches HEAD", func(t *testing.T) {
		reg, fake := newFakeRegistry(t, registryFiles())
		_, err := reg.Resolve(remoteURL, "", "data.txt")
		require.NoError(t, err)
		require.Equal(
			t,
			[]string{"-c protocol.version=2 fetch -q --depth 1 " + remoteURL + " HEAD"},
			fake.fetches,
		)
	})

	t.Run("fetch is memoized per repo and rev", func(t *testing.T) {
		reg, fake := newFakeRegistry(t, registryFiles())
		_, err := reg.Resolve(remoteURL, "v1", "data.txt")
		require.NoError(t, err)
		_, err = reg.Resolve(remoteURL, "v1", "data/dir")
		require.NoError(t, err)
		_, err = reg.Resolve(remoteURL, "v2", "data.txt")
		require.NoError(t, err)
		require.Len(t, fake.fetches, 2)
	})

	t.Run("fetch failure", func(t *testing.T) {
		reg, fake := newFakeRegistry(t, registryFiles())
		runGit = func(dir string, args ...string) ([]byte, error) {
			if args[0] == "-c" {
				return nil, fmt.Errorf("git fetch: fatal: couldn't find remote ref v9")
			}
			return fake.run(dir, args...)
		}
		_, err := reg.Resolve(remoteURL, "v9", "data.txt")
		require.ErrorContains(t, err, "fetch revision v9 from "+remoteURL)
		require.ErrorContains(t, err, "couldn't find remote ref v9")
	})
}

func TestRemoteResolution(t *testing.T) {
	withConfig := func(t *testing.T, cfg string) (*Registry, *fakeGit) {
		files := registryFiles()
		if cfg == "" {
			delete(files, sha+":.dud/config.yaml")
		} else {
			files[sha+":.dud/config.yaml"] = cfg
		}
		return newFakeRegistry(t, files)
	}

	t.Run("named remote, case-insensitive", func(t *testing.T) {
		reg, _ := withConfig(t, "remotes:\n  Local: /tmp/registry\nremote: local\n")
		got, err := reg.Resolve(remoteURL, "v1", "data.txt")
		require.NoError(t, err)
		require.Equal(t, "/tmp/registry", got.Remote)
	})

	t.Run("literal rclone path", func(t *testing.T) {
		reg, _ := withConfig(t, "remote: s3:legacy-bucket\n")
		got, err := reg.Resolve(remoteURL, "v1", "data.txt")
		require.NoError(t, err)
		require.Equal(t, "s3:legacy-bucket", got.Remote)
	})

	t.Run("no remote", func(t *testing.T) {
		reg, _ := withConfig(t, "cache: .dud/cache\n")
		_, err := reg.Resolve(remoteURL, "v1", "data.txt")
		require.EqualError(t, err, remoteURL+" declares no remote in .dud/config.yaml")
	})

	t.Run("no config file", func(t *testing.T) {
		reg, _ := withConfig(t, "")
		_, err := reg.Resolve(remoteURL, "v1", "data.txt")
		require.ErrorContains(t, err, "declares no remote")
	})
}

func TestIsLocalPath(t *testing.T) {
	cases := map[string]bool{
		"../registry":                      true,
		"/srv/registry":                    true,
		"registry":                         true,
		"./registry":                       true,
		"https://github.com/org/reg.git":   false,
		"ssh://git@github.com/org/reg.git": false,
		"git@github.com:org/reg.git":       false,
		"file:///srv/registry":             false,
		"host:path/with/slash":             false,
		"/path/with:colon/after/slash":     true,
		"relative/path/with:colon":         true,
	}
	for repo, want := range cases {
		require.Equal(t, want, IsLocalPath(repo), repo)
	}
}

// TestResolveRealGit drives the package against an actual git repository to
// make sure the fetch/rev-parse/show plumbing holds together.
func TestResolveRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := runGit(dir, args...)
		require.NoError(t, err)
		return strings.TrimSpace(string(out))
	}
	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	git("init", "-q")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "test")
	write(".dud/index", "data.txt.yaml\n")
	write(".dud/config.yaml", "remotes:\n  fake: /tmp/fake_remote\nremote: fake\n")
	write("data.txt.yaml", "outputs:\n  data.txt:\n    checksum: v1checksum\n")
	git("add", "-A")
	git("commit", "-q", "-m", "v1")
	v1 := git("rev-parse", "HEAD")
	// An annotated tag must be peeled to its commit.
	git("tag", "-a", "-m", "release", "v1")
	write("data.txt.yaml", "outputs:\n  data.txt:\n    checksum: v2checksum\n")
	git("commit", "-q", "-am", "v2")
	v2 := git("rev-parse", "HEAD")

	reg, err := New(agglog.NewNullLogger())
	require.NoError(t, err)
	defer reg.Close()

	got, err := reg.Resolve(dir, "v1", "data.txt")
	require.NoError(t, err)
	require.Equal(t, v1, got.RevLock)
	require.Equal(t, "v1checksum", got.Artifact.Checksum)
	require.Equal(t, "/tmp/fake_remote", got.Remote)

	got, err = reg.Resolve(dir, "", "data.txt")
	require.NoError(t, err)
	require.Equal(t, v2, got.RevLock)
	require.Equal(t, "v2checksum", got.Artifact.Checksum)

	got, err = reg.Resolve(dir, v1, "data.txt")
	require.NoError(t, err)
	require.Equal(t, v1, got.RevLock)

	_, err = reg.Resolve(dir, "v9", "data.txt")
	require.ErrorContains(t, err, "fetch revision v9")

	// A relative local path is resolved against the current directory, not
	// the scratch repository.
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Dir(dir)))
	defer os.Chdir(cwd)
	got, err = reg.Resolve(filepath.Base(dir), "v1", "data.txt")
	require.NoError(t, err)
	require.Equal(t, v1, got.RevLock)
}
