/*
Copyright © 2026 Gustavo Miranda
*/
package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const repoSlug = "gustmrg/devlog"

var assumeYes bool

// newUpdater builds an updater that verifies downloaded assets against the
// checksums.txt published alongside each GoReleaser release.
func newUpdater() (*selfupdate.Updater, error) {
	return selfupdate.NewUpdater(selfupdate.Config{
		Validator: &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
	})
}

// detectLatest queries the GitHub repository for the newest release.
func detectLatest(ctx context.Context) (*selfupdate.Release, bool, error) {
	updater, err := newUpdater()
	if err != nil {
		return nil, false, err
	}
	return updater.DetectLatest(ctx, selfupdate.ParseSlug(repoSlug))
}

// confirm prompts the user for a yes/no answer, defaulting to no.
func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update devlog to the latest release",
	Long: `Checks GitHub for a newer release and, if found, downloads it,
verifies its checksum, and replaces the running binary in place.

Options:
  -y, --yes   Skip the confirmation prompt`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if versionInfo == "dev" {
			return fmt.Errorf("cannot self-update a dev build; install a released binary first")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		updater, err := newUpdater()
		if err != nil {
			return fmt.Errorf("%s %w", color.RedString("✗"), err)
		}

		latest, found, err := updater.DetectLatest(ctx, selfupdate.ParseSlug(repoSlug))
		if err != nil {
			return fmt.Errorf("%s failed to check for updates: %w", color.RedString("✗"), err)
		}
		if !found {
			return fmt.Errorf("%s no release found for %s", color.RedString("✗"), repoSlug)
		}

		if latest.LessOrEqual(versionInfo) {
			fmt.Printf("%s already on the latest version (%s)\n", color.GreenString("✔"), versionInfo)
			return nil
		}

		fmt.Printf("Update available: %s → %s\n", versionInfo, latest.Version())
		if !assumeYes && !confirm("Download and install now?") {
			fmt.Println("Update cancelled.")
			return nil
		}

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("%s could not locate current binary: %w", color.RedString("✗"), err)
		}

		if err := updater.UpdateTo(ctx, latest, exe); err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return fmt.Errorf("%s update failed: no write permission for %s\n  try re-running with elevated permissions (e.g. sudo) or via your original install method", color.RedString("✗"), exe)
			}
			return fmt.Errorf("%s update failed: %w", color.RedString("✗"), err)
		}

		fmt.Printf("%s updated to %s\n", color.GreenString("✔"), latest.Version())
		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVarP(&assumeYes, "yes", "y", false, "Skip the confirmation prompt")
}
