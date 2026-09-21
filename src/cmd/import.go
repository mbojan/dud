package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kevin-hanselman/dud/src/artifact"
	"github.com/kevin-hanselman/dud/src/cache"
	"github.com/kevin-hanselman/dud/src/fsutil"
	"github.com/kevin-hanselman/dud/src/index"
	"github.com/kevin-hanselman/dud/src/registry"
	"github.com/kevin-hanselman/dud/src/stage"
	"github.com/kevin-hanselman/dud/src/strategy"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	importRev string
	importOut string
	// noFetch is shared by import and update.
	noFetch bool
)

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().StringVar(
		&importRev,
		"rev",
		"",
		"git revision (tag, branch or commit) of the registry to import from "+
			"(default: the registry's default branch)",
	)
	importCmd.Flags().StringVarP(
		&importOut,
		"out",
		"o",
		"",
		"where to put the artifact (default: the artifact's base name in the "+
			"current directory)",
	)
	importCmd.Flags().BoolVarP(
		&useCopyStrategy,
		"copy",
		"c",
		false,
		"copy the artifact instead of linking",
	)
	importCmd.Flags().BoolVar(
		&noFetch,
		"no-fetch",
		false,
		"only write the stage file; don't fetch and checkout the artifact",
	)
}

var importCmd = &cobra.Command{
	Use:   "import [flags] <repo> <path>",
	Short: "Import an artifact from a data registry",
	Long: `Import pins an artifact from a data registry and brings it into the workspace.

A data registry is any Dud project tracked with git whose committed artifacts
have been pushed to a remote. Import resolves <path> in the registry at <repo>
(a git URL or a local path), records which stage owns it and what its checksum
is, and writes an import stage named <out>.yaml next to the artifact:

    import:
      repo: git@github.com:org/data-registry.git
      rev: v2          # from --rev; empty means the registry's default branch
      rev-lock: 6c73…  # the commit 'rev' resolved to
      path: data/raw.csv
      remote: s3:registry-bucket
    outputs:
      raw.csv:
        checksum: 7de9…

An import stage is a frozen copy of the registry artifact: 'dud commit' and
'dud push' skip it, 'dud fetch' downloads it from the registry's remote
instead of the project's, and other stages may use it as an input like any
other artifact. Run 'dud update' to move an import to a newer revision, or
edit 'rev' in the stage file and then run 'dud update'.

Unless --no-fetch is given, the artifact is fetched into the cache and
checked out right away. Fetching goes through rclone using this project's
rclone config, so the remote named in the registry's config must also be
defined here (a plain directory path needs no configuration).`,
	Example: `dud import git@github.com:org/data-registry.git data/raw.csv --rev v2
dud import ../registry models/model.bin -o data/model.bin`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runImport(args[0], args[1]); err != nil {
			fatal(err)
		}
	},
}

func runImport(repo, artPath string) error {
	artPath = filepath.Clean(artPath)
	// prepare() changes into the project root, so a local registry path
	// given relative to the current directory must be resolved first. It is
	// stored relative to the project root like every other path in a stage.
	var err error
	if registry.IsLocalPath(repo) {
		if repo, err = filepath.Abs(repo); err != nil {
			return err
		}
	}

	out := importOut
	if out == "" {
		out = filepath.Base(artPath)
	}
	// Only the output goes through prepare(); the repo is not a project path.
	paths := []string{out}
	rootDir, ch, idx, err := prepare(paths)
	if err != nil {
		return err
	}
	out = paths[0]
	stagePath := out + ".yaml"

	if registry.IsLocalPath(repo) {
		if repo, err = filepath.Rel(rootDir, repo); err != nil {
			return err
		}
	}

	for _, path := range []string{stagePath, out} {
		exists, err := fsutil.Exists(path, false)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%s already exists", path)
		}
	}

	reg, err := registry.New(logger)
	if err != nil {
		return err
	}
	defer reg.Close()

	lookup, err := reg.Resolve(repo, importRev, artPath)
	if err != nil {
		return err
	}

	art := lookup.Artifact
	art.Path = out
	stg := stage.Stage{
		// FromFile cleans an empty working dir to ".", so the checksum has
		// to be computed on the form the stage will be reloaded in.
		WorkingDir: ".",
		Import: &stage.ImportSpec{
			Repo:    repo,
			Rev:     importRev,
			RevLock: lookup.RevLock,
			Path:    artPath,
			Remote:  lookup.Remote,
		},
		Outputs: map[string]*artifact.Artifact{out: &art},
	}
	if stg.Checksum, err = stg.CalculateChecksum(); err != nil {
		return err
	}
	if err := stg.Validate(stagePath); err != nil {
		return err
	}
	// AddStage rejects outputs already owned by another stage before
	// anything is written.
	if err := idx.AddStage(stg, stagePath); err != nil {
		return err
	}
	if err := writeNewStage(&stg, stagePath); err != nil {
		return err
	}
	if err := idx.ToFile(indexPath); err != nil {
		return err
	}
	logger.Info.Printf("added import stage %s\n", stagePath)

	if noFetch {
		return nil
	}
	return fetchAndCheckout(idx, ch, rootDir, []string{stagePath})
}

// writeNewStage serializes a stage to a path that must not exist yet.
func writeNewStage(stg *stage.Stage, stagePath string) error {
	file, err := os.OpenFile(stagePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if err != nil {
		return errors.Wrap(err, "writing stage "+stagePath)
	}
	defer file.Close()
	return errors.Wrap(stg.Serialize(file), "writing stage "+stagePath)
}

// fetchAndCheckout brings the outputs of the given (import) stages into the
// cache and the workspace. It deliberately does not go through the fetch and
// checkout commands, which would re-run prepare().
func fetchAndCheckout(
	idx index.Index,
	ch cache.LocalCache,
	rootDir string,
	stagePaths []string,
) error {
	strat := strategy.LinkStrategy
	if useCopyStrategy {
		strat = strategy.CopyStrategy
	}
	fetched := make(map[string]bool)
	for _, stagePath := range stagePaths {
		if err := idx.Fetch(
			stagePath,
			ch,
			rootDir,
			false,
			"",
			fetched,
			make(map[string]bool),
			logger,
		); err != nil {
			return err
		}
	}
	checkedOut := make(map[string]bool)
	for _, stagePath := range stagePaths {
		if err := idx.Checkout(
			stagePath,
			ch,
			rootDir,
			strat,
			false,
			checkedOut,
			make(map[string]bool),
			logger,
		); err != nil {
			return err
		}
	}
	return nil
}
