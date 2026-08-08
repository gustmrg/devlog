package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	gh "github.com/google/go-github/v74/github"
)

// Activity holds everything a user did on GitHub on a given date.
type Activity struct {
	Commits []CommitActivity
	PRs     []PRActivity
	Reviews []PRActivity
}

// CommitActivity is a single authored commit with its change stats.
type CommitActivity struct {
	SHA     string
	Repo    string // "owner/repo"
	Message string // first line of the commit message
	URL     string
}

// PRActivity is a pull request the user authored or reviewed.
type PRActivity struct {
	Number int
	Repo   string // "owner/repo"
	Title  string
	Action string // "opened", "merged" or "reviewed"
	URL    string
}

// searchOpts requests the maximum page size; a single page (100 items) is
// plenty for one day of activity.
var searchOpts = &gh.SearchOptions{ListOptions: gh.ListOptions{PerPage: 100}}

// FetchActivity collects the user's commits, authored PRs and PR reviews
// for the given date using the GitHub search API. Private repositories are
// included as long as the client's token has access to them.
func FetchActivity(ctx context.Context, client *gh.Client, username string, date time.Time) (Activity, error) {
	if username == "" {
		return Activity{}, fmt.Errorf("github.username is not set in the devlog config")
	}

	d := date.Format("2006-01-02")

	var activity Activity

	commits, err := fetchCommits(ctx, client, username, d)
	if err != nil {
		return activity, err
	}
	activity.Commits = commits

	prs, err := fetchAuthoredPRs(ctx, client, username, d)
	if err != nil {
		return activity, err
	}
	activity.PRs = prs

	reviews, err := fetchReviews(ctx, client, username, d)
	if err != nil {
		return activity, err
	}
	activity.Reviews = reviews

	return activity, nil
}

func fetchCommits(ctx context.Context, client *gh.Client, username, date string) ([]CommitActivity, error) {
	query := fmt.Sprintf("author:%s committer-date:%s", username, date)
	result, _, err := client.Search.Commits(ctx, query, searchOpts)
	if err != nil {
		return nil, fmt.Errorf("error searching commits: %w", err)
	}

	var commits []CommitActivity
	for _, item := range result.Commits {
		commits = append(commits, CommitActivity{
			SHA:     item.GetSHA(),
			Repo:    item.GetRepository().GetFullName(),
			Message: firstLine(item.GetCommit().GetMessage()),
			URL:     item.GetHTMLURL(),
		})
	}
	return commits, nil
}

func fetchAuthoredPRs(ctx context.Context, client *gh.Client, username, date string) ([]PRActivity, error) {
	queries := []struct {
		q      string
		action string
	}{
		{fmt.Sprintf("type:pr author:%s created:%s", username, date), "opened"},
		{fmt.Sprintf("type:pr author:%s merged:%s", username, date), "merged"},
	}

	var prs []PRActivity
	seen := map[string]bool{}
	for _, query := range queries {
		result, _, err := client.Search.Issues(ctx, query.q, searchOpts)
		if err != nil {
			return nil, fmt.Errorf("error searching pull requests: %w", err)
		}
		for _, issue := range result.Issues {
			pr := prFromIssue(issue, query.action)
			key := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
			if seen[key] {
				continue
			}
			seen[key] = true
			prs = append(prs, pr)
		}
	}
	return prs, nil
}

func fetchReviews(ctx context.Context, client *gh.Client, username, date string) ([]PRActivity, error) {
	query := fmt.Sprintf("type:pr reviewed-by:%s -author:%s updated:%s", username, username, date)
	result, _, err := client.Search.Issues(ctx, query, searchOpts)
	if err != nil {
		return nil, fmt.Errorf("error searching reviewed pull requests: %w", err)
	}

	var reviews []PRActivity
	for _, issue := range result.Issues {
		reviews = append(reviews, prFromIssue(issue, "reviewed"))
	}
	return reviews, nil
}

func prFromIssue(issue *gh.Issue, action string) PRActivity {
	repo := ""
	if u := issue.GetRepositoryURL(); u != "" {
		if i := strings.LastIndex(u, "/repos/"); i >= 0 {
			repo = u[i+len("/repos/"):]
		}
	}
	return PRActivity{
		Number: issue.GetNumber(),
		Repo:   repo,
		Title:  issue.GetTitle(),
		Action: action,
		URL:    issue.GetHTMLURL(),
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
