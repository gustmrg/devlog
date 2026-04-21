/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package summary

import (
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List previously generated summaries",
	Long: `Lists previously generated summaries from ~/.devlog/summaries/.

Options:
  -w, --week                Show summaries from the current week
  -m, --month               Show summaries from the current month
      --from <YYYY-MM-DD>   Start of date range
      --to <YYYY-MM-DD>     End of date range

Examples:
  devlog summary list
  devlog summary list --week
  devlog summary list --month
  devlog summary list --from 2026-04-01 --to 2026-04-14`,
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func init() {
	listCmd.Flags().BoolP("week", "w", false, "Show summaries from the current week")
	listCmd.Flags().BoolP("month", "m", false, "Show summaries from the current month")
	listCmd.Flags().String("from", "", "Start of date range (YYYY-MM-DD)")
	listCmd.Flags().String("to", "", "End of date range (YYYY-MM-DD)")
}
