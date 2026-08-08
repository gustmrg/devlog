package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ghsync "devlog/internal/github"
	"devlog/internal/llm"
	"devlog/internal/store"
	"devlog/templates"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Import GitHub activity (commits, PRs, reviews) as entries",
	Long: `Fetches your GitHub activity for a date — commits, pull requests you
authored and pull requests you reviewed — and adds them as entries to
that day's log. Works with private repositories the token can access.

Requires github.username in the config and a token in the environment
variable named by github.tokenEnvVar (default GITHUB_TOKEN). The token
needs the "repo" scope (classic) or Contents read access (fine-grained).

Re-running sync for the same date is safe: already-imported items are
skipped.

Examples:
  devlog sync
  devlog sync --date 2026-08-05
  devlog sync --polish
  devlog sync --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		dateFlag, _ := cmd.Flags().GetString("date")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		polish, _ := cmd.Flags().GetBool("polish")

		syncDate := time.Now()
		if dateFlag != "" {
			parsed, err := time.Parse("2006-01-02", dateFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s invalid date format, expected YYYY-MM-DD\n", color.RedString("✗"))
				return
			}
			syncDate = parsed
		}

		username := viper.GetString("github.username")
		if username == "" {
			fmt.Fprintf(os.Stderr, "%s github.username is not set — add it to ~/.devlog/config.json\n", color.RedString("✗"))
			return
		}

		tokenEnvVar := viper.GetString("github.tokenEnvVar")
		if tokenEnvVar == "" {
			tokenEnvVar = "GITHUB_TOKEN"
		}
		token := os.Getenv(tokenEnvVar)
		if token == "" {
			fmt.Fprintf(os.Stderr, "%s no token found — set the %s environment variable\n", color.RedString("✗"), tokenEnvVar)
			return
		}

		ctx := context.Background()
		client := ghsync.NewClient(ctx, token)

		fmt.Printf("Fetching GitHub activity for %s (%s)...\n", username, syncDate.Format("2006-01-02"))
		activity, err := ghsync.FetchActivity(ctx, client, username, syncDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}

		entries := mapActivity(activity)
		if len(entries) == 0 {
			fmt.Println("No GitHub activity found for this date.")
			return
		}

		if polish {
			polished, err := polishEntries(cmd.Context(), entries)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s polish failed, keeping raw descriptions: %s\n", color.YellowString("!"), err)
			} else {
				entries = polished
			}
		}

		if dryRun {
			for _, e := range entries {
				fmt.Printf("%s [%s] %s\n", color.CyanString("•"), e.Project, e.Description)
			}
			fmt.Printf("%s %d entries (dry-run, nothing written)\n", color.GreenString("✔"), len(entries))
			return
		}

		home, err := store.ConfigPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}
		entriesDir := filepath.Join(home, "entries")
		if err := os.MkdirAll(entriesDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "%s could not create entries directory: %s\n", color.RedString("✗"), err)
			return
		}

		logFile := filepath.Join(entriesDir, syncDate.Format("2006-01-02")+".json")
		dailyLog, err := store.LoadDailyLog(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}
		dailyLog.Date = syncDate.Format("2006-01-02")

		existing := map[string]bool{}
		for _, e := range dailyLog.Entries {
			if e.Source != "" {
				existing[e.Source] = true
			}
		}

		added := 0
		for _, e := range entries {
			if existing[e.Source] {
				continue
			}
			dailyLog.Entries = append(dailyLog.Entries, e)
			added++
			fmt.Printf("%s [%s] %s\n", color.GreenString("+"), e.Project, e.Description)
		}

		if added == 0 {
			fmt.Println("All activity for this date was already imported.")
			return
		}

		if err := store.SaveDailyLog(logFile, dailyLog); err != nil {
			fmt.Fprintf(os.Stderr, "%s %s\n", color.RedString("✗"), err)
			return
		}

		fmt.Printf("%s %d new entries added\n", color.GreenString("✔"), added)
	},
}

// mapActivity converts fetched GitHub activity into store entries.
func mapActivity(activity ghsync.Activity) []store.Entry {
	var entries []store.Entry
	now := time.Now()

	newEntry := func(project, description, source string, tags ...string) store.Entry {
		return store.Entry{
			Id:          uuid.NewString(),
			Project:     project,
			Description: description,
			Tags:        tags,
			CreatedAt:   now,
			Source:      source,
		}
	}

	for _, c := range activity.Commits {
		entries = append(entries, newEntry(shortRepoName(c.Repo), c.Message, "github:commit:"+c.SHA, "github", "commit"))
	}

	for _, pr := range activity.PRs {
		action := strings.ToUpper(pr.Action[:1]) + pr.Action[1:]
		description := fmt.Sprintf("%s PR #%d: %s", action, pr.Number, pr.Title)
		entries = append(entries, newEntry(shortRepoName(pr.Repo), description, fmt.Sprintf("github:pr:%s#%d", pr.Repo, pr.Number), "github", "pr"))
	}

	for _, pr := range activity.Reviews {
		description := fmt.Sprintf("Reviewed PR #%d: %s", pr.Number, pr.Title)
		entries = append(entries, newEntry(shortRepoName(pr.Repo), description, fmt.Sprintf("github:review:%s#%d", pr.Repo, pr.Number), "github", "review"))
	}

	return entries
}

// shortRepoName turns "owner/repo" into "repo" so entries from the same
// repository share one project name regardless of the item type.
func shortRepoName(fullName string) string {
	if i := strings.LastIndex(fullName, "/"); i >= 0 {
		return fullName[i+1:]
	}
	return fullName
}

// polishEntries rewrites entry descriptions with an LLM using the concise
// style template. It returns rewritten entries, or an error if the LLM
// output cannot be mapped back one-to-one onto the input.
func polishEntries(ctx context.Context, entries []store.Entry) ([]store.Entry, error) {
	template, err := templates.Get("concise")
	if err != nil {
		return nil, err
	}

	client, err := llm.NewFromConfig()
	if err != nil {
		return nil, err
	}

	var userPrompt strings.Builder
	language := viper.GetString("defaults.language")
	if language != "" {
		userPrompt.WriteString("Write every rewritten entry in " + language + ". ")
	}
	userPrompt.WriteString("Rewrite each raw git activity line below as a devlog entry in the style described. ")
	userPrompt.WriteString("Keep the same order and the same number of items. ")
	userPrompt.WriteString("Respond with a strict JSON array of strings and nothing else — no Markdown fences, no commentary.\n\n")
	for i, e := range entries {
		fmt.Fprintf(&userPrompt, "%d. [%s] %s\n", i+1, e.Project, e.Description)
	}

	reply, err := client.Complete(ctx, template, userPrompt.String())
	if err != nil {
		return nil, err
	}

	reply = strings.TrimSpace(reply)
	reply = strings.TrimPrefix(reply, "```json")
	reply = strings.TrimPrefix(reply, "```")
	reply = strings.TrimSuffix(reply, "```")
	reply = strings.TrimSpace(reply)

	var descriptions []string
	if err := json.Unmarshal([]byte(reply), &descriptions); err != nil {
		return nil, fmt.Errorf("error parsing polished entries: %w", err)
	}
	if len(descriptions) != len(entries) {
		return nil, fmt.Errorf("LLM returned %d entries, expected %d", len(descriptions), len(entries))
	}

	for i := range entries {
		if trimmed := strings.TrimSpace(descriptions[i]); trimmed != "" {
			entries[i].Description = trimmed
		}
	}
	return entries, nil
}

func init() {
	RootCmd.AddCommand(syncCmd)
	syncCmd.Flags().String("date", "", "Date to sync (YYYY-MM-DD, defaults to today)")
	syncCmd.Flags().Bool("dry-run", false, "Print what would be imported without writing entries")
	syncCmd.Flags().Bool("polish", false, "Rewrite descriptions with an LLM (requires llm.enabled in config)")
}
