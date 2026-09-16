package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	pushCmd.Flags().BoolVarP(
		&disableRecursion,
		"single-stage",
		"s",
		false,
		"disable recursive operation on upstream stages",
	)
	pushCmd.Flags().StringVarP(
		&remoteName,
		"remote",
		"r",
		"",
		"name of the remote to push to (see 'remotes' in the config)",
	)
	rootCmd.AddCommand(pushCmd)
}

var pushCmd = &cobra.Command{
	Use:   "push [flags] [remote] [stage_file]...",
	Short: "Push committed artifacts to a remote cache",
	Long: `Push uploads previously committed artifacts to a remote cache.

For each stage passed in, push uploads the stage's committed outputs to the
remote cache. If no stage files are passed in, push will act on all stages in
the index. By default, push will act recursively on all stages upstream of the
given stage(s).

Remotes are declared under 'remotes' in the Dud config file, and 'remote'
names the default. If the first argument is the name of a configured remote,
push uses that remote instead of the default; the --remote flag does the same
without the ambiguity with stage file names.

This command requires rclone to be installed on your machine. Visit
https://rclone.org/ for more information and installation instructions.`,
	Run: func(cmd *cobra.Command, args []string) {
		// prepare() rewrites args in place; remoteFromArgs needs the originals.
		rawArgs := append([]string(nil), args...)
		rootDir, ch, idx, err := prepare(args)
		if err != nil {
			fatal(err)
		}

		remote, paths, err := remoteFromArgs(rawArgs, args)
		if err != nil {
			fatal(err)
		}

		if len(idx) == 0 {
			fatal(emptyIndexError{})
		}

		if len(paths) == 0 {
			// Ignore disableRecursion flag when no args passed.
			disableRecursion = false
			for path := range idx {
				paths = append(paths, path)
			}
		}

		pushed := make(map[string]bool)
		for _, path := range paths {
			inProgress := make(map[string]bool)
			if err := idx.Push(
				path,
				ch,
				rootDir,
				!disableRecursion,
				remote,
				pushed,
				inProgress,
				logger,
			); err != nil {
				fatal(err)
			}
			logger.Info.Println()
		}
	},
}
