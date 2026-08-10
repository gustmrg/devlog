package entry

import (
	"devlog/internal/store"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewAddCmd() *cobra.Command {
	var date string

	addCmd := &cobra.Command{
		Use:   "add <description>",
		Short: "Log a new activity entry",
		Long: `Logs a new activity entry to your devlog.

Options:
  -p, --project <name>      Project name (uses config default if omitted)
  -t, --tags <list>         Comma-separated tags
      --date <YYYY-MM-DD>   Override date (defaults to today)
  -i                        Interactive mode — prompts for each field

Examples:
  devlog add "Implemented refresh token rotation" -p echo -t backend,auth
  devlog entry add "Reviewed checkout API" -p shop --date 2026-04-14`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := store.OpenRepository()
			if err != nil {
				return err
			}

			var entryDate time.Time
			if date == "" {
				entryDate = time.Now()
			} else {
				entryDate, err = time.Parse("2006-01-02", date)
				if err != nil {
					return fmt.Errorf("invalid date format, expected YYYY-MM-DD")
				}
			}

			project, _ := cmd.Flags().GetString("project")
			if project == "" {
				project = viper.GetString("defaults.project")
			}

			var tags []string
			if rawTags, _ := cmd.Flags().GetString("tags"); rawTags != "" {
				for t := range strings.SplitSeq(rawTags, ",") {
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
				UpdatedAt:   time.Now(),
			}
			if _, err := repo.AddEntries(entryDate, []store.Entry{entry}); err != nil {
				return err
			}

			fmt.Printf("%s new entry successfully added\n", color.GreenString("✔"))
			return nil
		},
	}

	addCmd.Flags().StringP("project", "p", "", "Project name (uses config default if omitted)")
	addCmd.Flags().StringP("tags", "t", "", "Comma-separated tags")
	addCmd.Flags().StringVar(&date, "date", "", "Override date (YYYY-MM-DD, defaults to today)")
	addCmd.Flags().BoolP("interactive", "i", false, "Interactive mode")

	return addCmd
}
