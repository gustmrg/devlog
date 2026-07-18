package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"devlog/internal/config"
	"devlog/internal/server"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var dataDir, address, configPath string
	var allowInsecure bool
	cmd := &cobra.Command{Use: "serve", Short: "Run the central DevLog API, scheduler, web UI, and integrations", RunE: func(cmd *cobra.Command, args []string) error {
		if configPath == "" {
			configPath = filepath.Join(dataDir, "config.json")
		}
		cfg, err := config.Load(configPath)
		if err != nil {
			return err
		}
		publicURL := os.Getenv("DEVLOG_PUBLIC_URL")
		if !allowInsecure && !strings.HasPrefix(publicURL, "https://") {
			return fmt.Errorf("DEVLOG_PUBLIC_URL must use HTTPS; use --allow-insecure only for local development")
		}
		srv, err := server.New(cfg, dataDir, os.Getenv("DEVLOG_PAIRING_CODE"), os.Getenv("DEVLOG_ADMIN_PASSWORD"), publicURL)
		if err != nil {
			return err
		}
		defer srv.Close()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Fprintf(cmd.ErrOrStderr(), "Pairing code: %s\n", srv.PairingCode)
		return srv.Run(ctx, address)
	}}
	cmd.Flags().StringVar(&dataDir, "data-dir", "./data", "Server data directory")
	cmd.Flags().StringVar(&address, "listen", ":8080", "HTTP listen address")
	cmd.Flags().StringVar(&configPath, "config", "", "Server config file (defaults to <data-dir>/config.json)")
	cmd.Flags().BoolVar(&allowInsecure, "allow-insecure", false, "Allow a non-HTTPS public URL for local development")
	return cmd
}
