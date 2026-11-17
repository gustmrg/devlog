package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"devlog/internal/scheduler"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var collectScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Collect activity automatically at set times",
	Long: `Install a job that runs devlog collect at the given times, Monday to
Friday unless --every-day is set. It uses launchd on macOS, which runs a
collection missed while the Mac slept once it wakes; the user's crontab on
Linux; and Task Scheduler on Windows, which runs a missed collection as soon
as it can. Installing again replaces the job.

Scheduled jobs have a minimal environment. Use --env-file to load tokens
from a file containing KEY=VALUE lines that only its owner can read.`,
	Example: `  devlog collect schedule --at 12:15,18:15 --remote
  devlog collect schedule --at 12:20,18:20 --sources github --remote --env-file ~/.devlog/collect.env`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		at, _ := cmd.Flags().GetString("at")
		everyDay, _ := cmd.Flags().GetBool("every-day")
		remote, _ := cmd.Flags().GetBool("remote")
		envFile, _ := cmd.Flags().GetString("env-file")
		sources, err := cmd.Flags().GetStringSlice("sources")
		if err != nil {
			return err
		}
		times, err := scheduler.ParseTimes(at)
		if err != nil {
			return err
		}
		if _, err := configuredCollectors(sources); err != nil {
			return err
		}
		jobArgs := []string{"collect"}
		if len(sources) > 0 {
			jobArgs = append(jobArgs, "--sources", strings.Join(sources, ","))
		}
		if remote {
			jobArgs = append(jobArgs, "--remote")
		}
		installedAt, logFile, err := installSchedule("collect", times, !everyDay, jobArgs, envFile)
		if err != nil {
			return fmt.Errorf("could not install automatic collection: %w", err)
		}
		labels := make([]string, len(times))
		for i, clock := range times {
			labels[i] = clock.String()
		}
		days := "on weekdays"
		if everyDay {
			days = "daily"
		}
		fmt.Printf("Automatic collection installed for %s %s (%s).\n", strings.Join(labels, ", "), days, installedAt)
		fmt.Printf("Logs: %s\n", logFile)
		return nil
	},
}

var collectScheduleShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the installed automatic collection",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		definition, installed, err := scheduler.Show("collect")
		if err != nil {
			return fmt.Errorf("could not show automatic collection: %w", err)
		}
		if !installed {
			fmt.Println("No automatic collection is installed.")
			return nil
		}
		fmt.Println(definition)
		return nil
	},
}

var collectScheduleRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the installed automatic collection",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		removed, err := scheduler.Remove("collect")
		if err != nil {
			return fmt.Errorf("could not remove automatic collection: %w", err)
		}
		if !removed {
			fmt.Println("No automatic collection is installed.")
			return nil
		}
		fmt.Println("Automatic collection removed.")
		return nil
	},
}

// redirectOutput appends everything the command prints to path. Task
// Scheduler runs programs without a shell to redirect their output.
func redirectOutput(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("could not open log file %s: %w", path, err)
	}
	os.Stdout, os.Stderr = file, file
	color.Output, color.Error = file, file
	color.NoColor = true
	fmt.Fprintf(file, "\n%s devlog %s\n", time.Now().Format(time.RFC3339), strings.Join(os.Args[1:], " "))
	return nil
}

func init() {
	collectCmd.AddCommand(collectScheduleCmd)
	collectScheduleCmd.AddCommand(collectScheduleShowCmd, collectScheduleRemoveCmd)
	collectScheduleCmd.Flags().String("at", "12:15,18:15", "Comma-separated times in 24-hour HH:MM format")
	collectScheduleCmd.Flags().Bool("every-day", false, "Run on weekends too")
	collectScheduleCmd.Flags().StringSlice("sources", nil, "Comma-separated sources (defaults to collect.sources when the job runs)")
	collectScheduleCmd.Flags().Bool("remote", false, "Pull and push the configured Git remote around each collection")
	collectScheduleCmd.Flags().String("env-file", "", "Protected KEY=VALUE file containing tokens")
}
