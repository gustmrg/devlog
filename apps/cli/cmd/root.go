package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"devlog/cmd/entry"
	"devlog/cmd/summary"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string

	RootCmd = &cobra.Command{
		Use:          "devlog",
		Short:        "Track daily dev activities and generate timesheet summaries",
		SilenceUsage: true,
		Long: `DevLog is a developer memory system for the command line.
	
	Log activities throughout the day as you work, then generate a structured
	summary at the end of your session — ready to paste into a timesheet.
	
	  devlog add "Implemented JWT auth middleware" -p echo -t backend,auth
	  devlog list
	  devlog summary create --style concise`,
	}
)

func Execute(version, commit, date string) {
	setVersionInfo(version, commit, date)

	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.AddCommand(entry.EntryCmd)
	RootCmd.AddCommand(entry.NewAddCmd())
	RootCmd.AddCommand(entry.NewListCmd())
	RootCmd.AddCommand(summary.SummaryCmd)
	RootCmd.AddCommand(versionCmd)
	RootCmd.AddCommand(updateCmd)
	RootCmd.CompletionOptions.DisableDefaultCmd = true
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		config := filepath.Join(home, ".devlog")
		viper.AddConfigPath(config)
		viper.SetConfigType("json")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			fmt.Fprintf(os.Stderr, "Warning: could not load config file; using defaults: %v\n", err)
		}
	}
}
