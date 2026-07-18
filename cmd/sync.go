package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"devlog/internal/agent"
	"devlog/internal/config"
	"devlog/internal/syncapi"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	return &cobra.Command{Use: "sync", Short: "Synchronize queued local events with the central server", RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		cfg, err := config.Load(filepath.Join(home, ".devlog", "config.json"))
		if err != nil {
			return err
		}
		db, err := agent.Open(home)
		if err != nil {
			return err
		}
		defer db.Close()
		_, credentialsPath := agent.Paths(home)
		credentials, err := syncapi.LoadCredentials(credentialsPath)
		if err != nil {
			return fmt.Errorf("device is not connected: %w", err)
		}
		a := agent.Agent{Config: cfg, DB: db, Credentials: credentials}
		if err := a.Sync(cmd.Context()); err != nil {
			return err
		}
		pending, err := db.PendingEvents(cmd.Context(), 1)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Sync complete; pending events: %d\n", len(pending))
		return nil
	}}
}
