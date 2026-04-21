package entry

import (
	"devlog/internal/store"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var date string

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Log a new activity entry",
	Long: `Logs a new activity entry to your devlog.

Options:
  -p, --project <name>      Project name (uses config default if omitted)
  -t, --tags <list>         Comma-separated tags
      --date <YYYY-MM-DD>   Override date (defaults to today)
  -i                        Interactive mode — prompts for each field

Examples:
  devlog entry add "Implemented refresh token rotation" -p echo -t backend,auth
  devlog entry add -i`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
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

		var entryDate time.Time
		if date == "" {
			entryDate = time.Now()
		} else {
			entryDate, err = time.Parse("2006-01-02", date)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s invalid date format, expected YYYY-MM-DD\n", color.RedString("✗"))
				return
			}
		}

		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			project = viper.GetString("defaults.project")
		}

		var tags []string
		if rawTags, _ := cmd.Flags().GetString("tags"); rawTags != "" {
			for _, t := range strings.Split(rawTags, ",") {
				if trimmed := strings.TrimSpace(t); trimmed != "" {
					tags = append(tags, trimmed)
				}
			}
		}

		entry := store.Entry{
			Id:          uuid.NewString(),
			Project:     project,
			Description: strings.Join(args, " "),
			Tags:        tags,
			CreatedAt:   time.Now(),
		}

		logFile := filepath.Join(entriesDir, entryDate.Format("2006-01-02")+".json")

		dailyLog, err := store.LoadDailyLog(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}
		dailyLog.Date = entryDate.Format("2006-01-02")
		dailyLog.Entries = append(dailyLog.Entries, entry)

		if err := store.SaveDailyLog(logFile, dailyLog); err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}

		fmt.Printf("%s new entry successfully added\n", color.GreenString("✔"))
	},
}

func init() {
	addCmd.Flags().StringP("project", "p", "", "Project name (uses config default if omitted)")
	addCmd.Flags().StringP("tags", "t", "", "Comma-separated tags")
	addCmd.Flags().StringVar(&date, "date", "", "Override date (YYYY-MM-DD, defaults to today)")
	addCmd.Flags().BoolP("interactive", "i", false, "Interactive mode")
}
