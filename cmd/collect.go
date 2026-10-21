package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"devlog/internal/collector"
	"devlog/internal/collector/gitlocal"
	"devlog/internal/store"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// defaultCollectSources run when collect.sources is not configured.
var defaultCollectSources = []string{"git"}

// collectorFactories build each collector from the configuration.
var collectorFactories = map[string]func() collector.Collector{
	"git": func() collector.Collector {
		return &gitlocal.Collector{
			Roots:        configList("collect.git.roots"),
			AuthorEmails: configList("collect.git.authoremails"),
		}
	},
}

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "Collect development activity from this device",
	Long: `Collects development activity and stores it as entries on the day it
happened, without rewriting it:

  git     commits you authored on every branch, branch activity from the
          reflog, and uncommitted or unpushed work on the checked-out branch,
          in the repositories under collect.git.roots

Each entry keeps its repository, branch, ticket keys (collect.ticketPattern),
and time, so an agent can later turn the day into timesheet descriptions.
Running collect again is safe: stored entries are completed, not duplicated,
and snapshots such as branch activity are refreshed.

The sources default to collect.sources. Times use collect.timezone, or the
system time zone when it is empty.`,
	Example: `  devlog collect
  devlog collect --date 2026-08-17 --dry-run
  devlog collect --from 2026-08-17 --to 2026-08-21 --sources git
  devlog collect --remote`,
	Args: cobra.NoArgs,
	RunE: runCollect,
}

type collectReport struct {
	From       time.Time             `json:"from"`
	To         time.Time             `json:"to"`
	DryRun     bool                  `json:"dryRun,omitempty"`
	Collectors []collectorRunSummary `json:"collectors"`
}

type collectorRunSummary struct {
	Name      string        `json:"name"`
	Added     int           `json:"added"`
	Updated   int           `json:"updated"`
	Unchanged int           `json:"unchanged"`
	Skipped   string        `json:"skipped,omitempty"`
	Error     string        `json:"error,omitempty"`
	Entries   []store.Entry `json:"entries,omitempty"`
}

func runCollect(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	remoteSync, _ := cmd.Flags().GetBool("remote")
	asJSON, _ := cmd.Flags().GetBool("json")
	envFile, _ := cmd.Flags().GetString("env-file")
	if envFile != "" {
		path, err := absolutePath(envFile)
		if err != nil {
			return fmt.Errorf("could not resolve environment file %q: %w", envFile, err)
		}
		if err := loadEnvFile(path); err != nil {
			return fmt.Errorf("could not load environment file: %w", err)
		}
	}

	loc, err := collectLocation()
	if err != nil {
		return err
	}
	window, err := collectWindow(cmd, loc)
	if err != nil {
		return err
	}
	sources, err := cmd.Flags().GetStringSlice("sources")
	if err != nil {
		return err
	}
	collectors, err := configuredCollectors(sources)
	if err != nil {
		return err
	}
	tickets, err := ticketPattern()
	if err != nil {
		return err
	}

	repo, err := store.OpenRepository()
	if err != nil {
		return fmt.Errorf("could not open DevLog data: %w", err)
	}
	if remoteSync && !dryRun {
		if err := syncConfiguredRemote(cmd.Context(), repo); err != nil {
			return fmt.Errorf("remote pull failed: %w", err)
		}
	}
	deviceID, err := store.DeviceID(repo.ConfigRoot)
	if err != nil {
		return fmt.Errorf("could not read the device ID: %w", err)
	}
	deviceName := deviceName()

	results := collector.Run(cmd.Context(), collectors, window, collector.Options{
		DeviceID: deviceID, DeviceName: deviceName, Tickets: tickets,
	})

	report := collectReport{From: window.From, To: window.To, DryRun: dryRun}
	runs := map[string]store.CollectorStatus{}
	var failed []string
	now := time.Now()
	for _, result := range results {
		summary := collectorRunSummary{Name: result.Collector, Skipped: result.Skipped}
		status := store.CollectorStatus{LastRunAt: now, Skipped: result.Skipped}
		if dryRun {
			summary.Entries = slices.Clone(result.Entries)
			sort.SliceStable(summary.Entries, func(i, j int) bool {
				return summary.Entries[i].Activity.OccurredAt.Before(summary.Entries[j].Activity.OccurredAt)
			})
		} else if len(result.Entries) > 0 {
			upserted, err := repo.UpsertEntries(result.Entries, loc)
			if err != nil {
				return fmt.Errorf("could not save %s activity: %w", result.Collector, err)
			}
			summary.Added, summary.Updated, summary.Unchanged = len(upserted.Added), len(upserted.Updated), upserted.Unchanged
		}
		status.Entries = len(result.Entries)
		if result.Err != nil {
			summary.Error = result.Err.Error()
			status.Error = summary.Error
			failed = append(failed, result.Collector)
		} else if result.Skipped == "" {
			coveredTo := window.To
			if now.Before(coveredTo) {
				coveredTo = now
			}
			status.LastSuccessAt, status.CoveredFrom, status.CoveredTo = now, window.From, coveredTo
		}
		runs[result.Collector] = status
		report.Collectors = append(report.Collectors, summary)
	}

	if !dryRun {
		if err := repo.RecordCollectorRuns(deviceName, runs); err != nil {
			return err
		}
		if remoteSync {
			if err := syncConfiguredRemote(cmd.Context(), repo); err != nil {
				return fmt.Errorf("activity was saved locally, but remote push failed: %w", err)
			}
		}
	}

	if asJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("could not encode the collection report: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	} else {
		printCollectReport(report)
	}
	if len(failed) > 0 {
		return fmt.Errorf("collection failed for %s", strings.Join(failed, ", "))
	}
	return nil
}

func printCollectReport(report collectReport) {
	last := report.To.Add(-time.Nanosecond)
	if report.From.Format("2006-01-02") == last.Format("2006-01-02") {
		fmt.Printf("Activity for %s\n", report.From.Format("2006-01-02"))
	} else {
		fmt.Printf("Activity from %s to %s\n", report.From.Format("2006-01-02"), last.Format("2006-01-02"))
	}
	for _, summary := range report.Collectors {
		switch {
		case summary.Skipped != "":
			fmt.Printf("%s %s skipped: %s\n", color.YellowString("·"), summary.Name, summary.Skipped)
			continue
		case report.DryRun:
			for _, entry := range summary.Entries {
				fmt.Printf("%s [%s] %s %s\n", color.CyanString("•"), entry.Project, entry.Activity.OccurredAt.Format("15:04"), firstDescriptionLine(entry.Description))
			}
			fmt.Printf("%s %s: %d entries (dry-run, nothing written)\n", color.GreenString("✔"), summary.Name, len(summary.Entries))
		default:
			fmt.Printf("%s %s: %d new, %d updated, %d unchanged\n", color.GreenString("✔"), summary.Name, summary.Added, summary.Updated, summary.Unchanged)
		}
		if summary.Error != "" {
			fmt.Fprintf(os.Stderr, "%s %s failed: %s\n", color.RedString("✗"), summary.Name, summary.Error)
		}
	}
}

func firstDescriptionLine(description string) string {
	line, _, _ := strings.Cut(description, "\n")
	return line
}

// collectLocation is the time zone collected activity is filed under.
func collectLocation() (*time.Location, error) {
	name := strings.TrimSpace(viper.GetString("collect.timezone"))
	if name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("invalid collect.timezone %q: use an IANA name such as America/Los_Angeles", name)
	}
	return loc, nil
}

func collectWindow(cmd *cobra.Command, loc *time.Location) (collector.Window, error) {
	date, _ := cmd.Flags().GetString("date")
	from, _ := cmd.Flags().GetString("from")
	to, _ := cmd.Flags().GetString("to")
	parse := func(flag, value string) (time.Time, error) {
		parsed, err := time.ParseInLocation("2006-01-02", value, loc)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid --%s value %q: expected YYYY-MM-DD", flag, value)
		}
		return parsed, nil
	}
	switch {
	case date != "" && (from != "" || to != ""):
		return collector.Window{}, fmt.Errorf("use either --date or --from and --to")
	case date != "":
		day, err := parse("date", date)
		if err != nil {
			return collector.Window{}, err
		}
		return collector.DayWindow(day, loc), nil
	case from != "" || to != "":
		if from == "" || to == "" {
			return collector.Window{}, fmt.Errorf("--from and --to must be used together")
		}
		first, err := parse("from", from)
		if err != nil {
			return collector.Window{}, err
		}
		last, err := parse("to", to)
		if err != nil {
			return collector.Window{}, err
		}
		if last.Before(first) {
			return collector.Window{}, fmt.Errorf("--to must not be earlier than --from")
		}
		return collector.DaysWindow(first, last, loc), nil
	default:
		return collector.DayWindow(time.Now().In(loc), loc), nil
	}
}

func configuredCollectors(sources []string) ([]collector.Collector, error) {
	if len(sources) == 0 {
		sources = configList("collect.sources")
	}
	if len(sources) == 0 {
		sources = defaultCollectSources
	}
	var collectors []collector.Collector
	for _, source := range sources {
		name := strings.ToLower(strings.TrimSpace(source))
		factory, ok := collectorFactories[name]
		if !ok {
			known := make([]string, 0, len(collectorFactories))
			for candidate := range collectorFactories {
				known = append(known, candidate)
			}
			sort.Strings(known)
			return nil, fmt.Errorf("unknown activity source %q; choose from %s", source, strings.Join(known, ", "))
		}
		if !slices.ContainsFunc(collectors, func(c collector.Collector) bool { return c.Name() == name }) {
			collectors = append(collectors, factory())
		}
	}
	return collectors, nil
}

func ticketPattern() (*regexp.Regexp, error) {
	pattern := viper.GetString("collect.ticketpattern")
	if pattern == "" {
		pattern = collector.DefaultTicketPattern
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid collect.ticketPattern %q: %w", pattern, err)
	}
	return compiled, nil
}

// deviceName labels this device in collected activity.
func deviceName() string {
	if name := strings.TrimSpace(viper.GetString("device.name")); name != "" {
		return name
	}
	if host, err := os.Hostname(); err == nil {
		return host
	}
	return "unknown"
}

// configList reads a list setting saved as a JSON array or written by hand as
// a comma-separated string.
func configList(key string) []string {
	var values []string
	if value, ok := viper.Get(key).(string); ok {
		values = strings.Split(value, ",")
	} else {
		values = viper.GetStringSlice(key)
	}
	var cleaned []string
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func init() {
	RootCmd.AddCommand(collectCmd)
	collectCmd.Flags().String("date", "", "Day to collect (YYYY-MM-DD, defaults to today)")
	collectCmd.Flags().String("from", "", "First day to collect (YYYY-MM-DD), with --to")
	collectCmd.Flags().String("to", "", "Last day to collect (YYYY-MM-DD), with --from")
	collectCmd.Flags().StringSlice("sources", nil, "Comma-separated sources to collect (defaults to collect.sources)")
	collectCmd.Flags().Bool("dry-run", false, "Print what would be collected without writing entries")
	collectCmd.Flags().Bool("remote", false, "Pull and push the configured DevLog Git remote")
	collectCmd.Flags().Bool("json", false, "Print the collection report as JSON")
	collectCmd.Flags().String("env-file", "", "Load API credentials from a protected KEY=VALUE file")
	_ = collectCmd.Flags().MarkHidden("env-file")
}
