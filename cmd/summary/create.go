/*
Copyright © 2026 Gustavo Miranda
*/
package summary

import (
	"context"
	"devlog/internal/llm"
	"devlog/internal/store"
	"devlog/templates"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

		repo, err := store.OpenRepository()
		if err != nil {
			return fmt.Errorf("%s %s", color.RedString("✗"), err)
		}
		entries, err := repo.Entries(summaryDate)
		if err != nil {
			return fmt.Errorf("%s %s", color.RedString("✗"), err)
		}

		if len(entries) == 0 {
			fmt.Printf("  %s\n", color.New(color.FgHiBlack).Sprintf("· No entries found for %s", summaryDate.Format("2006-01-02")))
			return nil
		}

		grouped := groupByProject(entries)

		if ai && style == "" {
			style = viper.GetString("defaults.style")
		}
		if style == "" {
			style = "concise"
		}

		content := buildContent(grouped)
		aiGenerated := false

		if ai {
			aiContent, err := generateWithLLM(cmd.Context(), grouped, style)
			if err != nil {
				return fmt.Errorf("%s %s", color.RedString("✗"), err)
			}
			content = aiContent
			aiGenerated = true
		}

		summary := store.Summary{
			ID:          summaryDate.Format("2006-01-02"),
			Date:        summaryDate,
			Projects:    grouped,
			Style:       style,
			AIGenerated: aiGenerated,
			Content:     content,
		}

		if err := repo.SaveSummary(summary); err != nil {
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

// generateWithLLM produces a polished narrative summary from the day's
// entries using the prompt template for the given style.
func generateWithLLM(ctx context.Context, groups []store.ProjectGroup, style string) (string, error) {
	template, err := templates.Get(style)
	if err != nil {
		return "", err
	}

	client, err := llm.NewFromConfig()
	if err != nil {
		return "", err
	}

	var userPrompt strings.Builder
	language := viper.GetString("defaults.language")
	if language != "" {
		userPrompt.WriteString("Write the output in " + language + ". ")
	}
	userPrompt.WriteString("These are my devlog entries for the day, grouped by project. ")
	userPrompt.WriteString("Turn them into a work summary in the style described. ")
	userPrompt.WriteString("Output only Markdown: a bold project name as heading per project, followed by bullet points. No title, no preamble, no closing remarks.\n\n")
	for _, g := range groups {
		userPrompt.WriteString("Project: " + g.Name + "\n")
		for _, e := range g.Entries {
			userPrompt.WriteString("- " + e.Description + "\n")
		}
		userPrompt.WriteString("\n")
	}

	content, err := client.Complete(ctx, template, userPrompt.String())
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(content), nil
}

func init() {
	createCmd.Flags().StringVar(&date, "date", "", "Summarize a specific date (YYYY-MM-DD)")
	createCmd.Flags().Bool("ai", false, "Use an LLM to produce a polished narrative")
	createCmd.Flags().StringP("style", "s", "", "Output style: concise, detailed, formal, impersonal (requires --ai)")
}
