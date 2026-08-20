package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"devlog/internal/scheduler"
	"devlog/internal/store"

	"github.com/spf13/cobra"
)

var syncScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Install a daily automatic sync",
	Long: `Install a daily sync using launchd on macOS or the user's crontab on Linux.

Scheduled jobs have a minimal environment. Use --env-file to load API keys
from a file containing KEY=VALUE lines. The file must only be accessible by
its owner (for example, chmod 600 ~/.devlog/sync.env).`,
	Example: `  devlog sync schedule --daily-at 23:55
  devlog sync schedule --daily-at 18:00 --polish --remote --env-file ~/.devlog/sync.env`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dailyAt, _ := cmd.Flags().GetString("daily-at")
		polish, _ := cmd.Flags().GetBool("polish")
		remote, _ := cmd.Flags().GetBool("remote")
		envFile, _ := cmd.Flags().GetString("env-file")

		hour, minute, err := scheduler.ParseTime(dailyAt)
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("could not locate the DevLog executable: %w", err)
		}
		executable, err = filepath.Abs(executable)
		if err != nil {
			return err
		}
		if envFile != "" {
			envFile, err = absolutePath(envFile)
			if err != nil {
				return fmt.Errorf("could not resolve environment file: %w", err)
			}
			if err := validateEnvFile(envFile); err != nil {
				return err
			}
		}
		home, err := store.ConfigPath()
		if err != nil {
			return err
		}
		logFile := filepath.Join(home, "sync.log")
		installedAt, err := scheduler.Install(scheduler.Options{
			Executable: executable,
			Hour:       hour,
			Minute:     minute,
			Polish:     polish,
			Remote:     remote,
			EnvFile:    envFile,
			LogFile:    logFile,
		})
		if err != nil {
			return fmt.Errorf("could not install automatic sync: %w", err)
		}

		fmt.Printf("Automatic sync installed for %02d:%02d daily (%s).\n", hour, minute, installedAt)
		fmt.Printf("Logs: %s\n", logFile)
		if envFile == "" {
			fmt.Println("Note: ensure GITHUB_TOKEN and any LLM API key are available to the scheduled job, or reinstall with --env-file.")
		}
		return nil
	},
}

var syncScheduleShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the installed automatic sync definition",
	RunE: func(cmd *cobra.Command, args []string) error {
		definition, installed, err := scheduler.Show()
		if err != nil {
			return fmt.Errorf("could not show automatic sync: %w", err)
		}
		if !installed {
			fmt.Println("No automatic sync is installed.")
			return nil
		}
		fmt.Println(definition)
		return nil
	},
}

var syncScheduleRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the installed automatic sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		removed, err := scheduler.Remove()
		if err != nil {
			return fmt.Errorf("could not remove automatic sync: %w", err)
		}
		if !removed {
			fmt.Println("No automatic sync is installed.")
			return nil
		}
		fmt.Println("Automatic sync removed.")
		return nil
	},
}

func absolutePath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Abs(path)
}

func validateEnvFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("could not access environment file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("environment file %s is not a regular file", path)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("environment file permissions are too open; run chmod 600 %s", path)
	}
	return nil
}

func loadEnvFile(path string) error {
	if err := validateEnvFile(path); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("invalid environment line %q: expected KEY=VALUE", line)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("could not set %s: %w", key, err)
		}
	}
	return scanner.Err()
}

func init() {
	syncCmd.AddCommand(syncScheduleCmd)
	syncScheduleCmd.AddCommand(syncScheduleShowCmd, syncScheduleRemoveCmd)
	syncScheduleCmd.Flags().String("daily-at", "23:55", "Daily sync time in 24-hour HH:MM format")
	syncScheduleCmd.Flags().Bool("polish", false, "Rewrite synced descriptions with the configured LLM")
	syncScheduleCmd.Flags().Bool("remote", false, "Pull and push the configured Git remote around GitHub import")
	syncScheduleCmd.Flags().String("env-file", "", "Protected KEY=VALUE file containing API credentials")
}
