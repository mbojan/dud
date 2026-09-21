package cmd

import (
	"fmt"
	"os"

	"github.com/kevin-hanselman/dud/src/artifact"
	"github.com/kevin-hanselman/dud/src/cache"
	"github.com/kevin-hanselman/dud/src/fsutil"
	"github.com/kevin-hanselman/dud/src/index"
	"github.com/kevin-hanselman/dud/src/registry"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVarP(
		&useCopyStrategy,
		"copy",
		"c",
		false,
		"copy artifacts instead of linking",
	)
	updateCmd.Flags().BoolVar(
		&noFetch,
		"no-fetch",
		false,
		"only update the stage files; don't fetch and checkout the artifacts",
	)
}

var updateCmd = &cobra.Command{
	Use:   "update [flags] [stage_file]...",
	Short: "Update import stages to the latest revision of their registry",
	Long: `Update re-resolves import stages against their data registries.

For each import stage passed in (all import stages in the index if none are
given), update looks up the stage's 'path' in the registry at 'rev' (or the
registry's default branch when 'rev' is empty) and, if the artifact changed,
rewrites 'rev-lock', 'remote' and the output's checksum. To move an import to
another tag or branch, edit 'rev' in the stage file and run update. Stages
written by hand with only 'repo', 'path' and optionally 'rev' are completed
the same way.

Unless --no-fetch is given, updated artifacts are fetched and checked out.
The previous version is removed from the workspace first, but only if it is
unmodified; a modified artifact must be moved aside by hand.

See 'dud import' for a description of import stages.`,
	Run: func(cmd *cobra.Command, paths []string) {
		if err := runUpdate(paths); err != nil {
			fatal(err)
		}
	},
}

func runUpdate(paths []string) error {
	rootDir, ch, idx, err := prepare(paths)
	if err != nil {
		return err
	}

	if len(paths) == 0 {
		for _, stagePath := range idx.SortStagePaths() {
			if idx[stagePath].IsImport() {
				paths = append(paths, stagePath)
			}
		}
		if len(paths) == 0 {
			logger.Info.Println("no import stages in the index")
			return nil
		}
	}
	for _, stagePath := range paths {
		stg, ok := idx[stagePath]
		if !ok {
			return fmt.Errorf("unknown stage %#v", stagePath)
		}
		if !stg.IsImport() {
			return fmt.Errorf("stage %s is not an import stage", stagePath)
		}
	}

	reg, err := registry.New(logger)
	if err != nil {
		return err
	}
	defer reg.Close()

	var updated []string
	for _, stagePath := range paths {
		changed, err := updateStage(reg, idx, ch, rootDir, stagePath)
		if err != nil {
			return err
		}
		if changed {
			updated = append(updated, stagePath)
		}
	}
	if noFetch || len(updated) == 0 {
		return nil
	}
	return fetchAndCheckout(idx, ch, rootDir, updated)
}

// updateStage re-resolves one import stage and rewrites its file if the
// registry moved. It reports whether the stage's artifact changed.
func updateStage(
	reg *registry.Registry,
	idx index.Index,
	ch cache.LocalCache,
	rootDir string,
	stagePath string,
) (bool, error) {
	stg := idx[stagePath]
	lookup, err := reg.Resolve(stg.Import.Repo, stg.Import.Rev, stg.Import.Path)
	if err != nil {
		return false, err
	}

	// Validate guarantees exactly one output.
	var out *artifact.Artifact
	for _, art := range stg.Outputs {
		out = art
	}
	newArt := lookup.Artifact
	newArt.Path = out.Path

	checksum, err := stg.CalculateChecksum()
	if err != nil {
		return false, err
	}
	artChanged := *out != newArt
	if !artChanged &&
		stg.Import.RevLock == lookup.RevLock &&
		stg.Import.Remote == lookup.Remote &&
		stg.Checksum == checksum {
		logger.Info.Printf("stage %s is up to date\n", stagePath)
		return false, nil
	}

	if artChanged {
		if err := removeIfUnmodified(ch, rootDir, *out); err != nil {
			return false, err
		}
	}

	stg.Import.RevLock = lookup.RevLock
	stg.Import.Remote = lookup.Remote
	*out = newArt
	if stg.Checksum, err = stg.CalculateChecksum(); err != nil {
		return false, err
	}
	if err := stg.ToFile(stagePath); err != nil {
		return false, err
	}
	logger.Info.Printf("updated stage %s\n", stagePath)
	return artChanged, nil
}

// removeIfUnmodified clears an artifact from the workspace so a newer version
// can be checked out in its place. Dud never discards user changes
// implicitly, so anything that doesn't match the committed version is left
// for the user to deal with.
func removeIfUnmodified(ch cache.LocalCache, rootDir string, art artifact.Artifact) error {
	status, err := ch.Status(rootDir, art, true)
	if err != nil {
		return err
	}
	if status.WorkspaceFileStatus == fsutil.StatusAbsent {
		return nil
	}
	if !status.ContentsMatch {
		return fmt.Errorf(
			"%s differs from its imported version; move it aside before updating",
			art.Path,
		)
	}
	return os.RemoveAll(art.Path)
}
