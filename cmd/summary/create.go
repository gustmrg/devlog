/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package summary

import (
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Generate a structured summary from logged entries",
	Long: `Generates a structured summary from logged entries and saves it to ~/.devlog/summaries/.

Options:
      --date <YYYY-MM-DD>   Summarize a specific date (defaults to today)
  -w, --week                Generate a weekly summary
  -s, --style <style>       Output style: concise, detailed, formal, impersonal
      --ai                  Use an LLM to produce a polished narrative
  -f, --format <template>   Template from ~/.devlog/templates/

Examples:
  devlog summary create
  devlog summary create --style formal
  devlog summary create --week --style detailed
  devlog summary create --ai --style impersonal`,
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func init() {
	createCmd.Flags().String("date", "", "Summarize a specific date (YYYY-MM-DD)")
	createCmd.Flags().BoolP("week", "w", false, "Generate a weekly summary")
	createCmd.Flags().StringP("style", "s", "", "Output style: concise, detailed, formal, impersonal")
	createCmd.Flags().Bool("ai", false, "Use an LLM to produce a polished narrative")
	createCmd.Flags().StringP("format", "f", "", "Template from ~/.devlog/templates/")
}
