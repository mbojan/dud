package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(pullCmd)
	pullCmd.Flags().BoolVarP(
		&useCopyStrategy,
		"copy",
		"c",
		false,
		"copy artifacts instead of linking",
	)
	pullCmd.Flags().BoolVarP(
		&disableRecursion,
		"single-stage",
		"s",
		false,
		"don't operate recursively over Stage inputs",
	)
	pullCmd.Flags().StringVarP(
		&remoteName,
		"remote",
		"r",
		"",
		"name of the remote to fetch from (see 'remotes' in the config)",
	)
}

var pullCmd = &cobra.Command{
	Use:   "pull [flags] [remote] [stage_file]...",
	Short: "Fetch artifacts from a remote and checkout",
	Long: `Pull runs fetch followed by checkout.

Remotes are declared under 'remotes' in the Dud config file, and 'remote'
names the default. If the first argument is the name of a configured remote,
pull fetches from that remote instead of the default; the --remote flag does
the same without the ambiguity with stage file names.

This command requires rclone to be installed on your machine. Visit
https://rclone.org/ for more information and installation instructions.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch strips a leading remote name from the args; checkout must only
		// ever see stage paths.
		stagePaths := runFetch(args)
		// After fetch completes, remove its lock file so checkout can take
		// control.
		// TODO: Removing the lock file between operations is awkward and
		// probably buggy. We definitely should revisit this.
		if err := unlockProject(); err != nil {
			fatal(err)
		}
		checkoutCmd.Run(cmd, stagePaths)
	},
}
