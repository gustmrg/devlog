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
		
		// get directory and validate if it exists
		// get flag date and try to parse
		// get all the entries then filter
		home, err := store.ConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}

		entriesDir := filepath.Join(home, "entries")
		if err := os.MkdirAll(entriesDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "%s could not create entries directory: %s\n", color.RedString("✗"), err)
			return
		}

		logDate := ""
		today := time.Now().Format("2006-01-02")

		if date != "" {
			parsedDate, err := time.Parse("2006-01-02", date)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s invalid date format, expected YYYY-MM-DD\n", color.RedString("✗"))
				return
			}

			logDate = parsedDate.Format("2006-01-02")
		} else {
			logDate = today
		}

		// add rule to get all entries if the date was not passed
		logFile := filepath.Join(entriesDir, logDate + ".json")
		
		dailyLog, err := store.LoadDailyLog(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}

		entries := dailyLog.Entries

		if len(entries) == 0 {
			if logDate == today {
				fmt.Printf("No entries for today\n")	
				return
			} else {
				fmt.Printf("No entries for %s\n", logDate)
				return
			}
		}

		fmt.Printf("%s\n", dailyLog.Date)
		for index := range entries {
			fmt.Printf("- %s: %s\n", entries[index].Project, entries[index].Description)
			
			if len(entries[index].Tags) > 0 {
				fmt.Printf("tags: %s\n", strings.Join(entries[index].Tags, ", "))
			} 
		}
	},
}

func init() {
	listCmd.Flags().StringVar(&date, "date", "", "Show entries for a specific date (YYYY-MM-DD)")
	listCmd.Flags().BoolP("week", "w", false, "Show entries for the current week")
	listCmd.Flags().StringP("project", "p", "", "Filter by project")
	listCmd.Flags().String("tag", "", "Filter by tag")
}
