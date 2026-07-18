package entry

import (
	"devlog/internal/agent"
	"devlog/internal/config"
	"devlog/internal/store"
	"devlog/internal/syncapi"
	"encoding/json"
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

Examples:
  devlog list
  devlog list --week
  devlog list --project echo
  devlog entry list --date 2026-04-13`,
		Run: func(cmd *cobra.Command, args []string) {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
				return
			}

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

			db, err := agent.Open(home)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
				return
			}
			defer db.Close()
			events, err := db.EventsForDay(cmd.Context(), logDate, time.Local)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
				return
			}
			entries := make([]store.Entry, 0, len(events))
			for _, event := range events {
				var payload struct {
					Description string   `json:"description"`
					Tags        []string `json:"tags"`
				}
				_ = json.Unmarshal(event.Payload, &payload)
				description := payload.Description
				if description == "" {
					description = event.Kind
				}
				entries = append(entries, store.Entry{Id: event.ID, Project: event.ProjectID, Description: description, Tags: payload.Tags, CreatedAt: event.OccurredAt})
			}
			cfg, _ := config.Load(filepath.Join(home, ".devlog", "config.json"))
			_, credentialsPath := agent.Paths(home)
			credentials, credentialErr := syncapi.LoadCredentials(credentialsPath)
			if credentialErr == nil && cfg.Server.URL != "" {
				client := syncapi.Client{BaseURL: cfg.Server.URL, Token: credentials.Token}
				timeline, remoteErr := client.Timeline(cmd.Context(), logDate)
				if remoteErr == nil {
					if payload, marshalErr := json.Marshal(timeline); marshalErr == nil {
						_ = db.CacheTimeline(cmd.Context(), logDate, payload)
					}
				} else if cached, cacheErr := db.CachedTimeline(cmd.Context(), logDate); cacheErr == nil {
					_ = json.Unmarshal(cached, &timeline)
				}
				if len(timeline.Activities) > 0 {
					entries = entries[:0]
					for _, activity := range timeline.Activities {
						entries = append(entries, store.Entry{Id: activity.ID, Project: activity.ProjectID, Description: activity.Description, CreatedAt: activity.StartedAt})
					}
				}
			}

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
				meta = append(meta, dimColor.Sprint(e.CreatedAt.In(time.Local).Format("3:04 PM")))

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

	listCmd.Flags().StringVar(&date, "date", "", "Show entries for a specific date (YYYY-MM-DD)")
	listCmd.Flags().BoolP("week", "w", false, "Show entries for the current week")
	listCmd.Flags().StringP("project", "p", "", "Filter by project")
	listCmd.Flags().String("tag", "", "Filter by tag")

	return listCmd
}
