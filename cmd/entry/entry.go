package entry

import "github.com/spf13/cobra"

var EntryCmd = &cobra.Command{
	Use:   "entry",
	Short: "Manage log entries",
	Long:  `Add, edit, delete, and list devlog entries.`,
}

func init() {
	EntryCmd.AddCommand(addCmd)
	EntryCmd.AddCommand(listCmd)
}
