/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package summary

import (
	"github.com/spf13/cobra"
)

var date string

var SummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Create and view summaries of logged entries",
	Long: `Manage summaries generated from logged entries stored in ~/.devlog/summaries/.

Subcommands:
  create   Generate a new summary from logged entries
  list     List previously generated summaries
  show     Display the contents of a previously generated summary

Examples:
  devlog summary create
  devlog summary create --style formal
  devlog summary create --week --style detailed
  devlog summary create --ai --style impersonal
  devlog summary list
  devlog summary list --week
  devlog summary list --from 2026-04-01 --to 2026-04-14
  devlog summary show
  devlog summary show --date 2026-04-13
`,
}

func init() {
	SummaryCmd.AddCommand(createCmd)
	SummaryCmd.AddCommand(listCmd)
	SummaryCmd.AddCommand(showCmd)
}
