/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package summary

import (
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the contents of a previously generated summary",
	Long: `Displays the contents of a previously generated summary from ~/.devlog/summaries/.

Options:
      --date <YYYY-MM-DD>   Show summary for a specific date (defaults to today)

Examples:
  devlog summary show
  devlog summary show --date 2026-04-13`,
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func init() {
	showCmd.Flags().String("date", "", "Show summary for a specific date (YYYY-MM-DD)")
}
