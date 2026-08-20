/*
Copyright © 2026 Gustavo Miranda
*/
package summary

import (
	"devlog/internal/store"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List previously generated summaries",
	Long: `Lists previously generated summaries from ~/.devlog/summaries/.

With no flags, summaries from the current week (Monday–Sunday) are shown.

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
	RunE: func(cmd *cobra.Command, args []string) error {
		week, _ := cmd.Flags().GetBool("week")
		month, _ := cmd.Flags().GetBool("month")
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")

		hasRange := from != "" || to != ""
		switch {
		case week && month:
			return fmt.Errorf("--week and --month cannot be used together")
		case week && hasRange:
			return fmt.Errorf("--week cannot be used with --from or --to")
		case month && hasRange:
			return fmt.Errorf("--month cannot be used with --from or --to")
		}

		start, end, label, err := resolveRange(week, month, from, to)
		if err != nil {
			return err
		}

		repo, err := store.OpenRepository()
		if err != nil {
			return fmt.Errorf("could not open DevLog data: %w", err)
		}
		summaryFiles, err := repo.SummaryFiles()
		if err != nil {
			return fmt.Errorf("could not list saved summaries: %w", err)
		}

		var summaries []store.Summary
		for _, summaryFile := range summaryFiles {
			name := strings.TrimSuffix(filepath.Base(summaryFile), ".md")
			d, err := time.Parse("2006-01-02", name)
			if err != nil {
				continue
			}
			d = truncateDay(d)
			if d.Before(start) || d.After(end) {
				continue
			}

			summary, err := store.LoadSummary(summaryFile)
			if err != nil {
				return fmt.Errorf("could not load summary %s: %w", name, err)
			}
			summaries = append(summaries, summary)
		}

		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].Date.After(summaries[j].Date)
		})

		if len(summaries) == 0 {
			fmt.Printf("  %s\n", summaryLabelColor.Sprintf("· No summaries for %s", label))
			return nil
		}

		separator := summaryLabelColor.Sprint(strings.Repeat("─", 52))
		fmt.Printf("\n  %s\n", summaryHeaderColor.Sprintf("Summaries · %s", label))
		fmt.Printf("  %s\n\n", separator)

		for _, s := range summaries {
			projectNames := make([]string, len(s.Projects))
			for i, p := range s.Projects {
				projectNames[i] = summaryProjectColor.Sprint(p.Name)
			}

			date := s.Date.Format("2006-01-02")
			fmt.Printf("  %s  %s\n", date, summaryLabelColor.Sprint(s.Style))
			if len(projectNames) > 0 {
				indent := strings.Repeat(" ", 12)
				fmt.Printf("  %s%s\n", indent, strings.Join(projectNames, summaryLabelColor.Sprint(", ")))
			}
			fmt.Println()
		}

		fmt.Printf("  %s\n", separator)
		noun := "summary"
		if len(summaries) != 1 {
			noun = "summaries"
		}
		fmt.Printf("  %s\n\n", summaryLabelColor.Sprintf("%d %s", len(summaries), noun))

		return nil
	},
}

// truncateDay strips the time component, keeping only the calendar date.
func truncateDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// resolveRange computes the inclusive [start, end] date window and a human
// label describing it. With no flags it defaults to the current week.
func resolveRange(week, month bool, from, to string) (time.Time, time.Time, string, error) {
	now := truncateDay(time.Now())

	switch {
	case month:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, -1)
		return start, end, now.Format("January 2006"), nil

	case from != "" || to != "":
		start := time.Time{}
		end := time.Date(9999, 12, 31, 0, 0, 0, 0, now.Location())
		if from != "" {
			parsed, err := time.Parse("2006-01-02", from)
			if err != nil {
				return time.Time{}, time.Time{}, "", fmt.Errorf("invalid --from value %q: expected YYYY-MM-DD", from)
			}
			start = truncateDay(parsed)
		}
		if to != "" {
			parsed, err := time.Parse("2006-01-02", to)
			if err != nil {
				return time.Time{}, time.Time{}, "", fmt.Errorf("invalid --to value %q: expected YYYY-MM-DD", to)
			}
			end = truncateDay(parsed)
		}
		if start.After(end) {
			return time.Time{}, time.Time{}, "", fmt.Errorf("--from must not be after --to")
		}
		return start, end, rangeLabel(from, to), nil

	default: // week (explicit or default)
		// ISO week: Monday is the first day. time.Weekday() returns 0 for Sunday.
		offset := (int(now.Weekday()) + 6) % 7
		start := now.AddDate(0, 0, -offset)
		end := start.AddDate(0, 0, 6)
		return start, end, fmt.Sprintf("%s – %s", start.Format("2006-01-02"), end.Format("2006-01-02")), nil
	}
}

func rangeLabel(from, to string) string {
	switch {
	case from != "" && to != "":
		return fmt.Sprintf("%s – %s", from, to)
	case from != "":
		return fmt.Sprintf("since %s", from)
	default:
		return fmt.Sprintf("through %s", to)
	}
}

func init() {
	listCmd.Flags().BoolP("week", "w", false, "Show summaries from the current week")
	listCmd.Flags().BoolP("month", "m", false, "Show summaries from the current month")
	listCmd.Flags().String("from", "", "Start of date range (YYYY-MM-DD)")
	listCmd.Flags().String("to", "", "End of date range (YYYY-MM-DD)")
}
