package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	ghsync "devlog/internal/github"
	"devlog/internal/llm"
	"devlog/internal/store"
	"devlog/templates"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Import GitHub activity (commits, PRs, reviews) as entries",
	Long: `Fetches your GitHub activity for a date — commits, pull requests you
authored and pull requests you reviewed — and adds them as entries to
that day's log. Works with private repositories the token can access.

Uses github.username when configured, otherwise discovers the authenticated
GitHub login. Authentication comes from the environment variable named by
github.tokenEnvVar (default GITHUB_TOKEN), falling back to an authenticated
GitHub CLI. The token needs the "repo" scope (classic) or Contents and Pull
requests read access (fine-grained).

Re-running sync for the same date is safe: already-imported items are
skipped.

Examples:
  devlog sync
  devlog sync --date 2026-08-05
  devlog sync --polish
  devlog sync --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dateFlag, _ := cmd.Flags().GetString("date")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		polish, _ := cmd.Flags().GetBool("polish")
		remoteSync, _ := cmd.Flags().GetBool("remote")
		envFile, _ := cmd.Flags().GetString("env-file")
		if envFile != "" {
			path, err := absolutePath(envFile)
			if err != nil {
				return err
			}
			if err := loadEnvFile(path); err != nil {
				return fmt.Errorf("could not load environment file: %w", err)
			}
		}
		repo, err := store.OpenRepository()
		if err != nil {
			return err
		}
		if remoteSync && !dryRun {
			if err := syncConfiguredRemote(cmd.Context(), repo); err != nil {
				return fmt.Errorf("remote pull failed: %w", err)
			}
		}

		syncDate := time.Now()
		if dateFlag != "" {
			parsed, err := time.Parse("2006-01-02", dateFlag)
			if err != nil {
				return fmt.Errorf("invalid date format, expected YYYY-MM-DD")
			}
			syncDate = parsed
		}

		tokenEnvVar := viper.GetString("github.tokenEnvVar")
		if tokenEnvVar == "" {
			tokenEnvVar = "GITHUB_TOKEN"
		}
		token, tokenSource, err := resolveGitHubToken(cmd.Context(), tokenEnvVar)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		client := ghsync.NewClient(ctx, token)
		username := viper.GetString("github.username")
		if username == "" {
			user, _, err := client.REST.Users.Get(ctx, "")
			if err != nil {
				return fmt.Errorf("github.username is not set and the authenticated user could not be discovered: %w", err)
			}
			username = user.GetLogin()
			if username == "" {
				return fmt.Errorf("github.username is not set and GitHub returned an empty authenticated login")
			}
			fmt.Printf("Using authenticated GitHub user %s.\n", username)
		}
		if tokenSource == "gh" {
			fmt.Println("Using authentication from GitHub CLI.")
		}

		fmt.Printf("Fetching GitHub activity for %s (%s)...\n", username, syncDate.Format("2006-01-02"))
		activity, err := ghsync.FetchActivity(ctx, client, username, syncDate)
		if err != nil {
			return err
		}

		entries := mapActivity(activity)
		if len(entries) == 0 {
			fmt.Println("No GitHub activity found for this date.")
			return nil
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
			return nil
		}

		addedEntries, err := repo.AddEntries(syncDate, entries)
		if err != nil {
			return err
		}
		for _, e := range addedEntries {
			fmt.Printf("%s [%s] %s\n", color.GreenString("+"), e.Project, e.Description)
		}

		if len(addedEntries) == 0 {
			fmt.Println("All activity for this date was already imported.")
			return nil
		}

		fmt.Printf("%s %d new entries added\n", color.GreenString("✔"), len(addedEntries))
		if remoteSync {
			if err := syncConfiguredRemote(cmd.Context(), repo); err != nil {
				return fmt.Errorf("entries were saved locally, but remote push failed: %w", err)
			}
		}
		return nil
	},
}

func resolveGitHubToken(ctx context.Context, envVar string) (string, string, error) {
	if token := strings.TrimSpace(os.Getenv(envVar)); token != "" {
		return token, "environment", nil
	}

	path, err := exec.LookPath("gh")
	if err != nil {
		return "", "", fmt.Errorf("no token found in %s and GitHub CLI is not installed", envVar)
	}
	output, err := exec.CommandContext(ctx, path, "auth", "token").Output()
	if err != nil {
		return "", "", fmt.Errorf("no token found in %s and GitHub CLI is not authenticated; run gh auth login", envVar)
	}
	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", "", fmt.Errorf("GitHub CLI returned an empty authentication token")
	}
	return token, "gh", nil
}

// mapActivity converts fetched GitHub activity into store entries.
func mapActivity(activity ghsync.Activity) []store.Entry {
	var entries []store.Entry

	newEntry := func(project, description, source string, tags ...string) store.Entry {
		now := time.Now()
		return store.Entry{
			Id:          store.EntryID(source),
			Project:     project,
			Description: description,
			Tags:        tags,
			CreatedAt:   now,
			UpdatedAt:   now,
			Source:      source,
		}
	}

	for _, c := range activity.Commits {
		entries = append(entries, newEntry(shortRepoName(c.Repo), c.Message, "github:commit:"+c.SHA, "github", "commit"))
	}

	for _, pr := range activity.PRs {
		action := strings.ToUpper(pr.Action[:1]) + pr.Action[1:]
		description := fmt.Sprintf("%s PR #%d: %s", action, pr.Number, pr.Title)
		entries = append(entries, newEntry(shortRepoName(pr.Repo), description, fmt.Sprintf("github:pr:%s#%d:%s", pr.Repo, pr.Number, pr.Action), "github", "pr"))
	}

	for _, pr := range activity.Reviews {
		description := fmt.Sprintf("Reviewed PR #%d: %s", pr.Number, pr.Title)
		entries = append(entries, newEntry(shortRepoName(pr.Repo), description, fmt.Sprintf("github:review:%s#%d:%s", pr.Repo, pr.Number, pr.OccurredAt.UTC().Format(time.RFC3339Nano)), "github", "review"))
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
	syncCmd.Flags().Bool("remote", false, "Pull and push the configured DevLog Git remote")
	syncCmd.Flags().String("env-file", "", "Load API credentials from a protected KEY=VALUE file")
	_ = syncCmd.Flags().MarkHidden("env-file")
}
