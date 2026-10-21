package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	ghsync "devlog/internal/github"
	"devlog/internal/llm"
	"devlog/internal/store"
	"devlog/templates"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Import GitHub activity (commits, PRs, reviews) as entries",
	Long: `Collects your GitHub activity for a date and adds it to that day's log.
It is the same as devlog collect --sources github, with --polish to rewrite
new descriptions with the configured LLM:

  - commits you pushed to any branch in the last 30 days (GitHub keeps no
    older events), and commits on default branches at any date
  - pull requests you opened or that were merged, with their head branch
  - pull requests you reviewed, and the issues and pull requests you
    commented on
  - branches you created

Works with private repositories the token can access. Uses github.username
when configured, otherwise discovers the authenticated GitHub login.
Authentication comes from the environment variable named by
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
		loc, err := collectLocation()
		if err != nil {
			return err
		}
		window, err := collectWindow(cmd, loc)
		if err != nil {
			return err
		}
		options := collectOptions{window: window, sources: []string{"github"}}
		options.dryRun, _ = cmd.Flags().GetBool("dry-run")
		options.polish, _ = cmd.Flags().GetBool("polish")
		options.remote, _ = cmd.Flags().GetBool("remote")
		options.envFile, _ = cmd.Flags().GetString("env-file")
		options.logFile, _ = cmd.Flags().GetString("log-file")
		return executeCollect(cmd, options)
	},
}

// connectGitHub authenticates the GitHub collector and finds the login whose
// activity it reads.
func connectGitHub(ctx context.Context) (*ghsync.Client, string, error) {
	tokenEnvVar := viper.GetString("github.tokenEnvVar")
	if tokenEnvVar == "" {
		tokenEnvVar = "GITHUB_TOKEN"
	}
	token, tokenSource, err := resolveGitHubToken(ctx, tokenEnvVar)
	if err != nil {
		return nil, "", err
	}
	if tokenSource == "gh" {
		fmt.Fprintln(os.Stderr, "Using authentication from GitHub CLI.")
	}
	client := ghsync.NewClient(ctx, token)
	username := viper.GetString("github.username")
	if username == "" {
		user, _, err := client.REST.Users.Get(ctx, "")
		if err != nil {
			return nil, "", fmt.Errorf("could not determine the GitHub username for the authenticated account: %w; set it with devlog config set github.username <username>", err)
		}
		username = user.GetLogin()
		if username == "" {
			return nil, "", fmt.Errorf("GitHub returned an empty username; set one with devlog config set github.username <username>")
		}
		fmt.Fprintf(os.Stderr, "Using authenticated GitHub user %s.\n", username)
	}
	return client, username, nil
}

func resolveGitHubToken(ctx context.Context, envVar string) (string, string, error) {
	if token := strings.TrimSpace(os.Getenv(envVar)); token != "" {
		return token, "environment", nil
	}

	path, err := exec.LookPath("gh")
	if err != nil {
		return "", "", fmt.Errorf("GitHub authentication is required; set %s or install and authenticate the GitHub CLI", envVar)
	}
	output, err := exec.CommandContext(ctx, path, "auth", "token").Output()
	if err != nil {
		return "", "", fmt.Errorf("GitHub authentication is required; set %s or run gh auth login", envVar)
	}
	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", "", fmt.Errorf("GitHub CLI returned an empty token; run gh auth login again or set %s", envVar)
	}
	return token, "gh", nil
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
		return nil, fmt.Errorf("could not parse the LLM response as a JSON string array: %w", err)
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
	syncCmd.Flags().String("log-file", "", "Append all output to this file")
	_ = syncCmd.Flags().MarkHidden("log-file")
}
