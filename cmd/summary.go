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
	Short: "Generate a structured summary from logged entries",
	Long: `Generates a structured summary from logged entries and saves it to ~/.devlog/summaries/.

Options:
      --date <YYYY-MM-DD>   Summarize a specific date (defaults to today)
  -w, --week                Generate a weekly summary
  -s, --style <style>       Output style: concise, detailed, formal, impersonal
      --ai                  Use an LLM to produce a polished narrative
  -f, --format <template>   Template from ~/.devlog/templates/

Examples:
  devlog summary
  devlog summary --style formal
  devlog summary --week --style detailed
  devlog summary --ai --style impersonal`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("summary called")
	},
}

func init() {
	rootCmd.AddCommand(summaryCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// summaryCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// summaryCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
