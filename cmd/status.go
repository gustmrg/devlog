package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"devlog/internal/agent"
	"devlog/internal/config"
	"devlog/internal/syncapi"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show local queue and central server connectivity", RunE: func(cmd *cobra.Command, args []string) error {
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
		pending, err := db.PendingEvents(cmd.Context(), 10000)
		if err != nil {
			return err
		}
		_, credentialsPath := agent.Paths(home)
		credentials, credentialErr := syncapi.LoadCredentials(credentialsPath)
		fmt.Fprintf(cmd.OutOrStdout(), "Device: %s\nServer: %s\nPending events: %d\n", credentials.DeviceID, cfg.Server.URL, len(pending))
		if credentialErr != nil || cfg.Server.URL == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "Connection: not paired")
			return nil
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(cfg.Server.URL + "/healthz")
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Connection: unavailable (%v)\n", err)
			return nil
		}
		defer resp.Body.Close()
		fmt.Fprintf(cmd.OutOrStdout(), "Connection: %s\n", resp.Status)
		return nil
	}}
}
