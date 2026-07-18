package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devlog/internal/agent"
	"devlog/internal/config"
	"devlog/internal/syncapi"
	"github.com/spf13/cobra"
)

func newConnectCmd() *cobra.Command {
	var code, name string
	var allowInsecure bool
	cmd := &cobra.Command{Use: "connect <server-url>", Short: "Pair this device with a DevLog server", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		serverURL := strings.TrimRight(args[0], "/")
		parsed, err := url.Parse(serverURL)
		if err != nil {
			return err
		}
		if parsed.Scheme != "https" && !allowInsecure {
			return fmt.Errorf("HTTPS is required; use --allow-insecure only on a trusted development network")
		}
		if name == "" {
			name, _ = os.Hostname()
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		result, err := (&syncapi.Client{BaseURL: serverURL}).Pair(ctx, code, name)
		if err != nil {
			return err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		_, credentialsPath := agent.Paths(home)
		if err := syncapi.SaveCredentials(credentialsPath, syncapi.Credentials{DeviceID: result.DeviceID, Token: result.Token}); err != nil {
			return err
		}
		configPath := filepath.Join(home, ".devlog", "config.json")
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		cfg.Device = config.DeviceConfig{ID: result.DeviceID, Name: name}
		cfg.Server = config.ServerConfig{URL: serverURL, AllowInsecure: allowInsecure}
		if err := config.Save(configPath, cfg); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Connected device %s (%s)\n", name, result.DeviceID)
		return nil
	}}
	cmd.Flags().StringVar(&code, "pairing-code", "", "One-time pairing code")
	_ = cmd.MarkFlagRequired("pairing-code")
	cmd.Flags().StringVar(&name, "name", "", "Device name (defaults to hostname)")
	cmd.Flags().BoolVar(&allowInsecure, "allow-insecure", false, "Allow HTTP on a trusted development network")
	return cmd
}
