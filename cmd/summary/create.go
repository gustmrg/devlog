/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package summary

import (
	"devlog/internal/store"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Generate a structured summary from logged entries",
	Long: `Generates a structured summary from logged entries and saves it to ~/.devlog/summaries/.

Options:
      --date <YYYY-MM-DD>   Summarize a specific date (defaults to today)
      --ai                  Use an LLM to produce a polished narrative
  -s, --style <style>       Output style: concise, detailed, formal, impersonal (requires --ai)

Examples:
  devlog summary create
  devlog summary create --date 2026-04-13
  devlog summary create --ai --style formal`,
	RunE: func(cmd *cobra.Command, args []string) error {
		style, _ := cmd.Flags().GetString("style")
		ai, _ := cmd.Flags().GetBool("ai")

		if style != "" && !ai {
			return fmt.Errorf("--style can only be used together with --ai")
		}

		summaryDate, err := getParsedDate(date)
		if err != nil {
			return err
		}

		home, err := store.ConfigPath()
		if err != nil {
			return fmt.Errorf("%s %s", color.RedString("✗"), err)
		}

		logFile := filepath.Join(home, "entries", summaryDate.Format("2006-01-02")+".json")
		dailyLog, err := store.LoadDailyLog(logFile)
		if err != nil {
			return fmt.Errorf("%s %s", color.RedString("✗"), err)
		}

		if len(dailyLog.Entries) == 0 {
			fmt.Printf("  %s\n", color.New(color.FgHiBlack).Sprintf("· No entries found for %s", summaryDate.Format("2006-01-02")))
			return nil
		}

		grouped := groupByProject(dailyLog.Entries)
		content := buildContent(grouped)

		summariesDir := filepath.Join(home, "summaries")
		if err := os.MkdirAll(summariesDir, 0755); err != nil {
			return fmt.Errorf("%s failed to create summaries directory: %w", color.RedString("✗"), err)
		}

		summary := store.Summary{
			ID:       summaryDate.Format("2006-01-02"),
			Date:     summaryDate,
			Projects: grouped,
			Style:    "concise",
			Content:  content,
		}

		summaryFile := filepath.Join(summariesDir, summaryDate.Format("2006-01-02")+".md")
		if err := store.SaveSummary(summaryFile, summary); err != nil {
			return fmt.Errorf("%s %s", color.RedString("✗"), err)
		}

		fmt.Printf("  %s Summary saved for %s\n", color.GreenString("✔"), summaryDate.Format("2006-01-02"))
		return nil
	},
}

func groupByProject(entries []store.Entry) []store.ProjectGroup {
	order := []string{}
	index := map[string]int{}

	for _, e := range entries {
		if _, exists := index[e.Project]; !exists {
			index[e.Project] = len(order)
			order = append(order, e.Project)
		}
	}

	groups := make([]store.ProjectGroup, len(order))
	for i, name := range order {
		groups[i] = store.ProjectGroup{Name: name}
	}

	for _, e := range entries {
		i := index[e.Project]
		groups[i].Entries = append(groups[i].Entries, e)
	}

	return groups
}

func buildContent(groups []store.ProjectGroup) string {
	var sb strings.Builder

	for i, g := range groups {
		if len(groups) > 1 {
			sb.WriteString(fmt.Sprintf("**%s**\n", g.Name))
		}
		for _, e := range g.Entries {
			sb.WriteString(fmt.Sprintf("- %s\n", e.Description))
		}
		if len(groups) > 1 && i < len(groups)-1 {
			sb.WriteString("\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func init() {
	createCmd.Flags().StringVar(&date, "date", "", "Summarize a specific date (YYYY-MM-DD)")
	createCmd.Flags().Bool("ai", false, "Use an LLM to produce a polished narrative")
	createCmd.Flags().StringP("style", "s", "", "Output style: concise, detailed, formal, impersonal (requires --ai)")
}
