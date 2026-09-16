package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(fetchCmd)
	fetchCmd.Flags().BoolVarP(
		&disableRecursion,
		"single-stage",
		"s",
		false,
		"don't operate recursively over Stage inputs",
	)
	fetchCmd.Flags().StringVarP(
		&remoteName,
		"remote",
		"r",
		"",
		"name of the remote to fetch from (see 'remotes' in the config)",
	)
}

var fetchCmd = &cobra.Command{
	Use:   "fetch [flags] [remote] [stage_file]...",
	Short: "Fetch committed artifacts from a remote cache",
	Long: `Fetch downloads previously committed artifacts from a remote cache.

For each stage passed in, fetch downloads the stage's committed outputs from
the remote cache. If no stage files are passed in, fetch will act on all
stages in the index. By default, fetch will act recursively on all stages
upstream of the given stage(s).

Remotes are declared under 'remotes' in the Dud config file, and 'remote'
names the default. If the first argument is the name of a configured remote,
fetch uses that remote instead of the default; the --remote flag does the same
without the ambiguity with stage file names.

This command requires rclone to be installed on your machine. Visit
https://rclone.org/ for more information and installation instructions.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFetch(args)
	},
}

// runFetch does the work of fetchCmd. It returns the stage paths it acted on,
// relative to the project root and with any leading remote name removed, so
// pull can hand exactly those paths on to checkout.
func runFetch(args []string) []string {
	// prepare() rewrites args in place; remoteFromArgs needs the originals.
	rawArgs := append([]string(nil), args...)
	rootDir, ch, idx, err := prepare(args)
	if err != nil {
		fatal(err)
	}

	remote, stagePaths, err := remoteFromArgs(rawArgs, args)
	if err != nil {
		fatal(err)
	}

	paths := stagePaths
	if len(paths) == 0 {
		// Ignore disableRecursion flag when no args passed.
		disableRecursion = false
		for path := range idx {
			paths = append(paths, path)
		}
	}

	fetched := make(map[string]bool)
	for _, path := range paths {
		inProgress := make(map[string]bool)
		if err := idx.Fetch(
			path,
			ch,
			rootDir,
			!disableRecursion,
			remote,
			fetched,
			inProgress,
			logger,
		); err != nil {
			fatal(err)
		}
		logger.Info.Println()
	}
	return stagePaths
}
