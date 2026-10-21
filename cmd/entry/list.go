package entry

import (
	"devlog/internal/store"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	headerColor  = color.New(color.FgCyan, color.Bold)
	projectColor = color.New(color.FgGreen, color.Bold)
	dimColor     = color.New(color.FgHiBlack)
	tagColor     = color.New(color.FgMagenta)
)

func NewListCmd() *cobra.Command {
	var date string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Display logged entries with optional filters",
		Long: `Displays logged entries with optional filters.

Options:
      --date <YYYY-MM-DD>  Show entries for a specific date
  -w, --week               Show entries for the current week
  -p, --project <name>     Filter by project
      --tag <name>         Filter by tag
      --json               Print the entries as JSON

Examples:
  devlog list
  devlog list --week
  devlog list --project echo
  devlog entry list --date 2026-04-13
  devlog list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := store.OpenRepository()
			if err != nil {
				return fmt.Errorf("could not open DevLog data: %w", err)
			}

			today := time.Now().Format("2006-01-02")
			logDate := today

			if date != "" {
				parsedDate, err := time.Parse("2006-01-02", date)
				if err != nil {
					return fmt.Errorf("invalid --date value %q: expected YYYY-MM-DD", date)
				}
				logDate = parsedDate.Format("2006-01-02")
			}

			parsedLogDate, _ := time.Parse("2006-01-02", logDate)
			entries, err := repo.Entries(parsedLogDate)
			if err != nil {
				return fmt.Errorf("could not read entries for %s: %w", logDate, err)
			}

			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				if entries == nil {
					entries = []store.Entry{}
				}
				data, err := json.MarshalIndent(map[string]any{"date": logDate, "entries": entries}, "", "  ")
				if err != nil {
					return fmt.Errorf("could not encode entries: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			if len(entries) == 0 {
				label := logDate
				if logDate == today {
					label = "today"
				}
				fmt.Printf("%s No entries for %s\n", dimColor.Sprint("·"), label)
				return nil
			}

			t, _ := time.Parse("2006-01-02", logDate)
			separator := dimColor.Sprint(strings.Repeat("─", 52))

			fmt.Printf("\n  %s\n", headerColor.Sprint(t.Format("Monday, January 2 · 2006")))
			fmt.Printf("  %s\n\n", separator)

			for i, e := range entries {
				idx := dimColor.Sprintf("%2d", i+1)
				project := projectColor.Sprintf("%-14s", e.Project)
				fmt.Printf("  %s  %s  %s\n", idx, project, e.Description)

				var meta []string
				for _, tag := range e.Tags {
					meta = append(meta, tagColor.Sprintf("#%s", tag))
				}
				meta = append(meta, dimColor.Sprint(e.SortTime().Format("3:04 PM")))

				indent := strings.Repeat(" ", 22)
				fmt.Printf("  %s%s\n\n", indent, strings.Join(meta, "  "))
			}

			fmt.Printf("  %s\n", separator)
			noun := "entry"
			if len(entries) != 1 {
				noun = "entries"
			}
			fmt.Printf("  %s\n\n", dimColor.Sprintf("%d %s", len(entries), noun))
			return nil
		},
	}

	listCmd.Flags().StringVar(&date, "date", "", "Show entries for a specific date (YYYY-MM-DD)")
	listCmd.Flags().BoolP("week", "w", false, "Show entries for the current week")
	listCmd.Flags().StringP("project", "p", "", "Filter by project")
	listCmd.Flags().String("tag", "", "Filter by tag")
	listCmd.Flags().Bool("json", false, "Print the entries as JSON")

	return listCmd
}
