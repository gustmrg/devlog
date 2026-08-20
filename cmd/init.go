/*
Copyright © 2026 Gustavo Miranda
*/
package cmd

import (
	"devlog/internal/store"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the ~/.devlog/ directory and default config",
	Long: `Creates the ~/.devlog/ directory structure and a default config.json.

Safe to run multiple times — will not overwrite existing data.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := store.Initialize()
		if err != nil {
			return fmt.Errorf("could not initialize DevLog: %w", err)
		}

		if !result.Created {
			fmt.Fprintf(cmd.OutOrStdout(), "%s DevLog is already initialized. Configuration left unchanged at %s\n", color.GreenString("✔"), result.ConfigFile)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s DevLog initialized. Configuration created at %s\n", color.GreenString("✔"), result.ConfigFile)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(initCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
