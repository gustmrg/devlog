package cmd

import (
	"fmt"

	"devlog/internal/store"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate legacy daily logs to the current data format",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, result, err := store.MigrateRepository()
		if err != nil {
			return err
		}
		if result.AlreadyCurrent {
			fmt.Println("DevLog data is already using the current format.")
			return nil
		}
		fmt.Printf("Migrated %d entries and %d summaries.\n", result.MigratedEntries, result.MigratedSummaries)
		if result.BackupPath != "" {
			fmt.Printf("Backup: %s\n", result.BackupPath)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(migrateCmd)
}
