package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Number     int
	Repo       string // "owner/repo"
	Title      string
	Action     string // "opened", "merged" or "reviewed"
	URL        string
	OccurredAt time.Time
}

// FetchActivity collects the user's commits, authored PRs and PR reviews
// for the given date using the GitHub search API. Private repositories are
// included as long as the client's token has access to them.
func FetchActivity(ctx context.Context, client *Client, username string, date time.Time) (Activity, error) {
	if username == "" {
		return Activity{}, fmt.Errorf("no GitHub username is configured; run devlog config set github.username <username>")
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

func fetchCommits(ctx context.Context, client *Client, username, date string) ([]CommitActivity, error) {
	query := fmt.Sprintf("author:%s committer-date:%s", username, date)
	var commits []CommitActivity
	opts := &gh.SearchOptions{ListOptions: gh.ListOptions{PerPage: 100}}
	expected := 0
	for {
		result, response, err := client.REST.Search.Commits(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("could not search GitHub commits: %w", err)
		}
		if result.GetIncompleteResults() {
			return nil, fmt.Errorf("GitHub returned incomplete commit search results; try again later")
		}
		if expected == 0 {
			expected = result.GetTotal()
		}
		for _, item := range result.Commits {
			commits = append(commits, CommitActivity{
				SHA:     item.GetSHA(),
				Repo:    item.GetRepository().GetFullName(),
				Message: firstLine(item.GetCommit().GetMessage()),
				URL:     item.GetHTMLURL(),
			})
		}
		if response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}
	if len(commits) < expected {
		return nil, fmt.Errorf("GitHub returned only %d of %d commit search results; try again later", len(commits), expected)
	}
	return commits, nil
}

func fetchAuthoredPRs(ctx context.Context, client *Client, username, date string) ([]PRActivity, error) {
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
		opts := &gh.SearchOptions{ListOptions: gh.ListOptions{PerPage: 100}}
		collected := 0
		expected := 0
		for {
			result, response, err := client.REST.Search.Issues(ctx, query.q, opts)
			if err != nil {
				return nil, fmt.Errorf("could not search GitHub pull requests: %w", err)
			}
			if result.GetIncompleteResults() {
				return nil, fmt.Errorf("GitHub returned incomplete pull request search results; try again later")
			}
			if expected == 0 {
				expected = result.GetTotal()
			}
			collected += len(result.Issues)
			for _, issue := range result.Issues {
				pr := prFromIssue(issue, query.action)
				key := fmt.Sprintf("%s#%d:%s", pr.Repo, pr.Number, pr.Action)
				if seen[key] {
					continue
				}
				seen[key] = true
				prs = append(prs, pr)
			}
			if response.NextPage == 0 {
				break
			}
			opts.Page = response.NextPage
		}
		if collected < expected {
			return nil, fmt.Errorf("GitHub returned only %d of %d pull request search results; try again later", collected, expected)
		}
	}
	return prs, nil
}

func fetchReviews(ctx context.Context, client *Client, username, date string) ([]PRActivity, error) {
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("could not prepare the GitHub review search: %w", err)
	}
	from := parsedDate.UTC()
	to := from.AddDate(0, 0, 1)
	query := `query($login: String!, $from: DateTime!, $to: DateTime!, $after: String) {
  user(login: $login) {
    contributionsCollection(from: $from, to: $to) {
      pullRequestReviewContributions(first: 100, after: $after) {
        nodes {
          occurredAt
          pullRequest { number title url repository { nameWithOwner } }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`
	type graphQLError struct {
		Message string `json:"message"`
	}
	type contribution struct {
		OccurredAt  time.Time `json:"occurredAt"`
		PullRequest struct {
			Number     int    `json:"number"`
			Title      string `json:"title"`
			URL        string `json:"url"`
			Repository struct {
				NameWithOwner string `json:"nameWithOwner"`
			} `json:"repository"`
		} `json:"pullRequest"`
	}
	type graphQLResponse struct {
		Data struct {
			User *struct {
				ContributionsCollection struct {
					PullRequestReviewContributions struct {
						Nodes    []contribution `json:"nodes"`
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
					} `json:"pullRequestReviewContributions"`
				} `json:"contributionsCollection"`
			} `json:"user"`
		} `json:"data"`
		Errors []graphQLError `json:"errors"`
	}

	var reviews []PRActivity
	var after any
	for {
		payload, err := json.Marshal(map[string]any{
			"query": query,
			"variables": map[string]any{
				"login": username,
				"from":  from.Format(time.RFC3339),
				"to":    to.Format(time.RFC3339),
				"after": after,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("could not encode the GitHub review query: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.GraphQLURL, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("could not build the GitHub review request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := client.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("could not search reviewed GitHub pull requests: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("could not read the GitHub review response: %w", readErr)
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub review search failed with HTTP status %d", response.StatusCode)
		}
		var result graphQLResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("GitHub returned an invalid review response: %w", err)
		}
		if len(result.Errors) > 0 {
			return nil, fmt.Errorf("GitHub review search failed: %s", result.Errors[0].Message)
		}
		if result.Data.User == nil {
			return nil, fmt.Errorf("GitHub user %q was not found; check github.username", username)
		}
		connection := result.Data.User.ContributionsCollection.PullRequestReviewContributions
		for _, item := range connection.Nodes {
			if item.OccurredAt.Before(from) || !item.OccurredAt.Before(to) {
				continue
			}
			reviews = append(reviews, PRActivity{
				Number:     item.PullRequest.Number,
				Repo:       item.PullRequest.Repository.NameWithOwner,
				Title:      item.PullRequest.Title,
				Action:     "reviewed",
				URL:        item.PullRequest.URL,
				OccurredAt: item.OccurredAt,
			})
		}
		if !connection.PageInfo.HasNextPage {
			break
		}
		if connection.PageInfo.EndCursor == "" {
			return nil, fmt.Errorf("GitHub returned an incomplete review page; try again later")
		}
		after = connection.PageInfo.EndCursor
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
