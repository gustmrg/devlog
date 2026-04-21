package entry

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Display logged entries with optional filters",
	Long: `Displays logged entries with optional filters.

Options:
      --date <YYYY-MM-DD>  Show entries for a specific date
  -w, --week               Show entries for the current week
  -p, --project <name>     Filter by project
      --tag <name>         Filter by tag

Examples:
  devlog entry list
  devlog entry list --week
  devlog entry list --project echo
  devlog entry list --date 2026-04-13`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("list called")
	},
}

func init() {
	listCmd.Flags().String("date", "", "Show entries for a specific date (YYYY-MM-DD)")
	listCmd.Flags().BoolP("week", "w", false, "Show entries for the current week")
	listCmd.Flags().StringP("project", "p", "", "Filter by project")
	listCmd.Flags().String("tag", "", "Filter by tag")
}
