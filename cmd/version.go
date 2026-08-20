/*
Copyright © 2026 Gustavo Miranda
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	versionInfo = "dev"
	commitInfo  = "none"
	dateInfo    = "unknown"

	checkUpdate bool
)

// setVersionInfo wires the build information injected into main into the root
// command, enabling both `devlog --version` and `devlog version`.
func setVersionInfo(version, commit, date string) {
	if version != "" {
		versionInfo = version
	}
	if commit != "" {
		commitInfo = commit
	}
	if date != "" {
		dateInfo = date
	}

	RootCmd.Version = versionInfo
	RootCmd.SetVersionTemplate(versionString())
}

func versionString() string {
	return fmt.Sprintf("devlog %s\ncommit: %s\nbuilt:  %s\n", versionInfo, commitInfo, dateInfo)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the devlog version, commit, and build date",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(versionString())
		if checkUpdate {
			runVersionCheck()
		}
	},
}

// runVersionCheck reports whether a newer release is available. It is
// best-effort: any failure is a soft, non-fatal notice to stderr.
func runVersionCheck() {
	if versionInfo == "dev" {
		fmt.Fprintln(os.Stderr, "Update check skipped: development builds do not have a release version.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	latest, found, err := detectLatest(ctx)
	if err != nil || !found {
		fmt.Fprintln(os.Stderr, "Could not check for updates; try again later.")
		return
	}

	if latest.LessOrEqual(versionInfo) {
		fmt.Println("You're on the latest version.")
		return
	}

	fmt.Printf("\nA new version is available: %s → %s\n%s\n", versionInfo, latest.Version(), latest.URL)
}

func init() {
	versionCmd.Flags().BoolVar(&checkUpdate, "check", false, "Check GitHub for a newer release")
}
