package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devlog/internal/agent"
	"devlog/internal/database"
	"devlog/internal/domain"
	"devlog/internal/store"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "migrate", Short: "Import data from previous DevLog storage formats"}
	cmd.AddCommand(newMigrateLegacyCmd())
	return cmd
}
func newMigrateLegacyCmd() *cobra.Command {
	var dryRun, yes bool
	cmd := &cobra.Command{Use: "legacy", Short: "Import legacy JSON entries and Markdown summaries", RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		files, err := filepath.Glob(filepath.Join(home, ".devlog", "entries", "*.json"))
		if err != nil {
			return err
		}
		summaryFiles, _ := filepath.Glob(filepath.Join(home, ".devlog", "summaries", "*.md"))
		entryCount := 0
		for _, file := range files {
			daily, err := store.LoadDailyLog(file)
			if err != nil {
				return err
			}
			entryCount += len(daily.Entries)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Legacy import preview: %d entries and %d summaries\n", entryCount, len(summaryFiles))
		if dryRun {
			return nil
		}
		if !yes && !confirm("Import without deleting the original files?") {
			fmt.Fprintln(cmd.OutOrStdout(), "Import cancelled.")
			return nil
		}
		db, err := agent.Open(home)
		if err != nil {
			return err
		}
		defer db.Close()
		ctx := context.Background()
		imported := 0
		for _, file := range files {
			daily, err := store.LoadDailyLog(file)
			if err != nil {
				return err
			}
			for _, entry := range daily.Entries {
				if entry.Project != "" {
					if err := db.UpsertProject(ctx, domain.Project{ID: entry.Project, Name: entry.Project, Enabled: true, CreatedAt: time.Now().UTC()}); err != nil {
						return err
					}
				}
				event := legacyEvent(daily.Date, entry)
				if n, err := db.QueueEvents(ctx, []domain.Event{event}); err != nil {
					return err
				} else {
					imported += n
				}
			}
		}
		for _, file := range summaryFiles {
			legacy, err := store.LoadSummary(file)
			if err != nil {
				return err
			}
			date := legacy.Date.Format("2006-01-02")
			revision, err := db.NextSummaryRevision(ctx, date)
			if err != nil {
				return err
			}
			id := uuid.NewString()
			if err := db.SaveSummary(ctx, domain.Summary{ID: id, Date: date, Revision: revision, Content: legacy.Content, Status: "legacy", CreatedAt: time.Now().UTC()}); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
				return err
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Imported %d new entries; original files were preserved.\n", imported)
		return nil
	}}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without importing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Import without confirmation")
	return cmd
}
func legacyEvent(date string, entry store.Entry) domain.Event {
	occurred := entry.CreatedAt
	if occurred.IsZero() {
		occurred, _ = time.Parse("2006-01-02", date)
	}
	raw := fmt.Sprintf("%s|%s|%s|%s", entry.Id, entry.Project, entry.Description, occurred.UTC().Format(time.RFC3339Nano))
	sum := sha256.Sum256([]byte(raw))
	id := entry.Id
	if id == "" {
		id = uuid.NewString()
	}
	return domain.Event{ID: id, SourceType: "manual", SourceInstanceID: "legacy", ExternalID: id, ProjectID: entry.Project, Kind: "manual.entry", OccurredAt: occurred.UTC(), ObservedAt: time.Now().UTC(), Payload: database.EncodePayload(map[string]any{"description": entry.Description, "tags": entry.Tags}), Fingerprint: "legacy:" + hex.EncodeToString(sum[:])}
}
