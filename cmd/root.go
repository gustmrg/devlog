package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"devlog/cmd/entry"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	
	RootCmd = &cobra.Command{
		Use:   "devlog",
		Short: "Track daily dev activities and generate timesheet summaries",
		Long: `DevLog is a developer memory system for the command line.
	
	Log activities throughout the day as you work, then generate a structured
	summary at the end of your session — ready to paste into a timesheet.
	
	  devlog add "Implemented JWT auth middleware" -p echo -t backend,auth -d 45
	  devlog summary --style concise`,
	}
)

func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.AddCommand(entry.EntryCmd)
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

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
