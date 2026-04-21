package entry

import (
	"devlog/internal/store"
	"fmt"
	"os"
	"path/filepath"
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

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Display logged entries with optional filters",
	Long: `Displays logged entries with optional filters.

Options:
      --date <YYYY-MM-DD>  Show entries for a specific date
  -w, --week               Show entries for the current week
  -p, --project <name>     Filter by project
      --tag <name>         Filter by tag

Examples:
  devlog entry list
  devlog entry list --week
  devlog entry list --project echo
  devlog entry list --date 2026-04-13`,
	Run: func(cmd *cobra.Command, args []string) {
		home, err := store.ConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}

		entriesDir := filepath.Join(home, "entries")

		today := time.Now().Format("2006-01-02")
		logDate := today

		if date != "" {
			parsedDate, err := time.Parse("2006-01-02", date)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s invalid date format, expected YYYY-MM-DD\n", color.RedString("✗"))
				return
			}
			logDate = parsedDate.Format("2006-01-02")
		}

		logFile := filepath.Join(entriesDir, logDate+".json")

		dailyLog, err := store.LoadDailyLog(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}

		entries := dailyLog.Entries

		if len(entries) == 0 {
			label := logDate
			if logDate == today {
				label = "today"
			}
			fmt.Printf("%s No entries for %s\n", dimColor.Sprint("·"), label)
			return
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
			meta = append(meta, dimColor.Sprint(e.CreatedAt.Format("3:04 PM")))

			indent := strings.Repeat(" ", 22)
			fmt.Printf("  %s%s\n\n", indent, strings.Join(meta, "  "))
		}

		fmt.Printf("  %s\n", separator)
		noun := "entry"
		if len(entries) != 1 {
			noun = "entries"
		}
		fmt.Printf("  %s\n\n", dimColor.Sprintf("%d %s", len(entries), noun))
	},
}

func init() {
	listCmd.Flags().StringVar(&date, "date", "", "Show entries for a specific date (YYYY-MM-DD)")
	listCmd.Flags().BoolP("week", "w", false, "Show entries for the current week")
	listCmd.Flags().StringP("project", "p", "", "Filter by project")
	listCmd.Flags().String("tag", "", "Filter by tag")
}
