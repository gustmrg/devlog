/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// summaryCmd represents the summary command
var summaryCmd = &cobra.Command{
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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("summary called")
	},
}

func init() {
	RootCmd.AddCommand(summaryCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// summaryCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// summaryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
