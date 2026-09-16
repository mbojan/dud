package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	validFields      = []string{"cache", "remote", "rclone_config", "remotes"}
	targetUserConfig bool
)

// validateConfigField checks a `config get`/`config set` field name. Besides
// the flat fields, `remotes.<name>` addresses a single named remote. The
// whole `remotes` map can be read but not set, since viper would parse the
// value as a plain string and clobber the map.
func validateConfigField(field string, allowMap bool) error {
	if name, isRemote := strings.CutPrefix(field, "remotes."); isRemote {
		// Viper splits keys on dots, so a dotted name would nest further.
		if name == "" || strings.Contains(name, ".") {
			return fmt.Errorf("invalid remote name '%s'", name)
		}
		return nil
	}
	if field == "remotes" && !allowMap {
		return fmt.Errorf(
			"'remotes' is a map; set individual entries with remotes.<name>",
		)
	}
	for _, valid := range validFields {
		if field == valid {
			return nil
		}
	}
	return fmt.Errorf(
		"invalid argument; expected one of [%s] or remotes.<name>",
		strings.Join(validFields, ", "),
	)
}

func init() {
	configCmd := &cobra.Command{
		Use:   "config {get|set}",
		Short: "Print or modify fields in the config file",
		Long:  "Config prints or modifies fields in the config file",
	}

	configCmd.AddCommand(
		&cobra.Command{
			Use:       "get <config_field>",
			Short:     "Get the value of a field in the config file",
			Long:      "Get the value of a field in the config file",
			ValidArgs: validFields,
			Args: func(cmd *cobra.Command, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected one argument, got %d", len(args))
				}
				return validateConfigField(args[0], true)
			},
			Run: func(cmd *cobra.Command, args []string) {
				rootDir, err := getProjectRootDir()
				if err != nil {
					fatal(err)
				}
				if err := lockProject(rootDir); err != nil {
					fatal(err)
				}
				if err := readConfig(rootDir); err != nil {
					fatal(err)
				}
				logger.Info.Println(viper.Get(args[0]))
			},
		},
	)

	setCmd := &cobra.Command{
		Use:       "set <config_field> <new_value>",
		Short:     "Set the value of a field in the config file",
		Long:      "Set the value of a field in the config file",
		ValidArgs: validFields,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("expected two arguments, got %d", len(args))
			}
			return validateConfigField(args[0], false)
		},
		Run: func(cmd *cobra.Command, args []string) {
			var err error

			if targetUserConfig {
				_, err = readUserConfig()
			} else {
				var rootDir string
				rootDir, err = getProjectRootDir()
				if err != nil {
					fatal(err)
				}
				if err := lockProject(rootDir); err != nil {
					fatal(err)
				}
				_, err = readProjectConfig(rootDir)
			}

			if err != nil && !os.IsNotExist(err) {
				fatal(err)
			}
			viper.Set(args[0], args[1])
			if err := viper.WriteConfig(); err != nil {
				fatal(err)
			}
		},
	}
	setCmd.Flags().BoolVarP(
		&targetUserConfig,
		"user",
		"u",
		false,
		"target the user-level config file",
	)
	configCmd.AddCommand(setCmd)

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Long:  "Print the config file path",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			var (
				configPath string
				err        error
			)

			if targetUserConfig {
				configPath, err = readUserConfig()
			} else {
				var rootDir string
				rootDir, err = getProjectRootDir()
				if err != nil {
					fatal(err)
				}
				configPath, err = readProjectConfig(rootDir)
			}

			if err != nil && !os.IsNotExist(err) {
				fatal(err)
			}
			fmt.Println(configPath)
		},
	}
	pathCmd.Flags().BoolVarP(
		&targetUserConfig,
		"user",
		"u",
		false,
		"target the user-level config file",
	)
	configCmd.AddCommand(pathCmd)

	rootCmd.AddCommand(configCmd)
}
