package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	appconfig "devlog/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{Use: "config", Short: "Inspect and validate DevLog configuration"}

func init() {
	configCmd.AddCommand(&cobra.Command{Use: "show", Short: "Print the effective local configuration", RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cfg, err := appconfig.Load(filepath.Join(home, ".devlog", "config.json"))
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	}}, &cobra.Command{Use: "validate", Short: "Validate the local configuration", RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		if _, err := appconfig.Load(filepath.Join(home, ".devlog", "config.json")); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Configuration is valid.")
		return nil
	}})
	RootCmd.AddCommand(configCmd)
}
