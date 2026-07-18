package cmd

import (
	"fmt"
	"path/filepath"

	"devlog/internal/database"
	"github.com/spf13/cobra"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Server administration utilities"}
	cmd.AddCommand(newBackupCmd())
	return cmd
}
func newBackupCmd() *cobra.Command {
	var dataDir, output string
	cmd := &cobra.Command{Use: "backup", Short: "Create a consistent SQLite server backup", RunE: func(cmd *cobra.Command, args []string) error {
		if output == "" {
			return fmt.Errorf("--output is required")
		}
		absolute, err := filepath.Abs(output)
		if err != nil {
			return err
		}
		db, err := database.Open(filepath.Join(dataDir, "devlog.db"))
		if err != nil {
			return err
		}
		defer db.Close()
		if err := db.Backup(cmd.Context(), absolute); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Backup written to %s\n", absolute)
		return nil
	}}
	cmd.Flags().StringVar(&dataDir, "data-dir", "/data", "Server data directory")
	cmd.Flags().StringVar(&output, "output", "", "Backup destination")
	return cmd
}
