/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package summary

import (
	"devlog/internal/store"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	summaryHeaderColor  = color.New(color.FgCyan, color.Bold)
	summaryLabelColor   = color.New(color.FgHiBlack)
	summaryProjectColor = color.New(color.FgGreen, color.Bold)
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
	RunE: func(cmd *cobra.Command, args []string) error {
		summaryDate, err := getParsedDate(date)
		if err != nil {
			return err
		}

		home, err := store.ConfigPath()
		if err != nil {
			return fmt.Errorf("%s %s\n", color.RedString("✗"), err)
		}

		summaryPath := filepath.Join(home, "summaries", summaryDate.Format("2006-01-02")+".md")
		summary, err := store.LoadSummary(summaryPath)
		if err != nil {
			return fmt.Errorf("%s %s\n", color.RedString("✗"), err)
		}

		if summary.Content == "" {
			fmt.Printf("  %s\n", summaryLabelColor.Sprintf("· No summary found for %s", summaryDate.Format("2006-01-02")))
			return nil
		}

		separator := summaryLabelColor.Sprint(strings.Repeat("─", 52))
		projectNames := make([]string, len(summary.Projects))
		for i, p := range summary.Projects {
			projectNames[i] = summaryProjectColor.Sprint(p.Name)
		}

		fmt.Printf("\n  %s\n", summaryHeaderColor.Sprintf("Summary · %s", summary.Date.Format("Monday, January 2 · 2006")))
		fmt.Printf("  %s\n\n", separator)
		fmt.Printf("  %s  %s\n", summaryLabelColor.Sprint("Style   "), summary.Style)
		fmt.Printf("  %s  %s\n", summaryLabelColor.Sprint("Projects"), strings.Join(projectNames, summaryLabelColor.Sprint(", ")))
		fmt.Printf("\n  %s\n", summary.Content)
		fmt.Printf("\n  %s\n\n", separator)

		return nil
	},
}

func init() {
	showCmd.Flags().StringVar(&date, "date", "", "Show summary for a specific date (YYYY-MM-DD)")
}

func getParsedDate(date string) (time.Time, error) {
	if date == "" {
		return time.Now(), nil
	} else {
		var err error 
		parsedDate, err := time.Parse("2006-01-02", date)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date format, expected YYYY-MM-DD\n")
		}

		return parsedDate, nil
	}
}
