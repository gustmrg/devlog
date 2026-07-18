package entry

import (
	"devlog/internal/agent"
	"devlog/internal/database"
	"devlog/internal/domain"
	"devlog/internal/store"
	"fmt"
	"os"
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
		Run: func(cmd *cobra.Command, args []string) {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
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
			}

			db, err := agent.Open(home)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
				return
			}
			defer db.Close()
			_ = db.UpsertProject(cmd.Context(), domain.Project{ID: project, Name: project, Enabled: true, CreatedAt: time.Now().UTC()})
			occurredAt := entry.CreatedAt
			if date != "" {
				occurredAt = time.Date(entryDate.Year(), entryDate.Month(), entryDate.Day(), 12, 0, 0, 0, time.Local)
			}
			event := domain.Event{ID: entry.Id, SourceType: "manual", SourceInstanceID: "cli", ExternalID: entry.Id, ProjectID: project, Kind: "manual.entry", OccurredAt: occurredAt.UTC(), ObservedAt: time.Now().UTC(), Payload: database.EncodePayload(map[string]any{"description": entry.Description, "tags": entry.Tags}), Fingerprint: "manual:" + entry.Id}
			if _, err := db.QueueEvents(cmd.Context(), []domain.Event{event}); err != nil {
				fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
				return
			}

			fmt.Printf("%s new entry successfully added\n", color.GreenString("✔"))
		},
	}

	addCmd.Flags().StringP("project", "p", "", "Project name (uses config default if omitted)")
	addCmd.Flags().StringP("tags", "t", "", "Comma-separated tags")
	addCmd.Flags().StringVar(&date, "date", "", "Override date (YYYY-MM-DD, defaults to today)")
	addCmd.Flags().BoolP("interactive", "i", false, "Interactive mode")

	return addCmd
}
