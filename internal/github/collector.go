package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"devlog/internal/collector"
	"devlog/internal/store"

	gh "github.com/google/go-github/v74/github"
)

const (
	// maxEventPages bounds the events read: GitHub keeps at most 300 events
	// from the last 30 days for a user.
	maxEventPages  = 3
	maxCommitPages = 3
	maxBodyLength  = 1500
)

// Collector gathers the user's GitHub activity: commits pushed to any branch,
// pull requests opened and merged, reviews, comments, and created branches.
type Collector struct {
	// Connect returns an authenticated client and the login to collect.
	Connect func(ctx context.Context) (*Client, string, error)
}

func (c *Collector) Name() string { return "github" }

// Collect combines the user's event feed, which sees pushes to every branch
// but only covers the last 30 days, with the search API, which finds commits
// on default branches and pull requests at any date.
func (c *Collector) Collect(ctx context.Context, request collector.Request) ([]store.Entry, error) {
	client, username, err := c.Connect(ctx)
	if err != nil {
		return nil, err
	}
	window := request.Window
	var entries []store.Entry
	seen := map[string]bool{}
	add := func(entry store.Entry) {
		if !seen[entry.Source] {
			seen[entry.Source] = true
			entries = append(entries, entry)
		}
	}

	events, err := fetchEvents(ctx, client, username, window.From)
	if err != nil {
		return nil, err
	}
	pushed, pushErr := pushedCommits(ctx, client, username, events, window)
	for _, entry := range pushed {
		add(entry)
	}
	for _, entry := range eventEntries(events, window) {
		add(entry)
	}

	heads := map[string]string{}
	for _, day := range utcDays(window) {
		activity, err := FetchActivity(ctx, client, username, day)
		if err != nil {
			return entries, errors.Join(pushErr, err)
		}
		for _, commit := range activity.Commits {
			if !commit.Generated && window.Contains(commit.AuthoredAt) {
				add(commitEntry(commit.Repo, "", commit.SHA, commit.Message, commit.Body, commit.URL, commit.AuthoredAt))
			}
		}
		for _, pr := range activity.PRs {
			if window.Contains(pr.OccurredAt) {
				add(prEntry(pr, pullRequestHead(ctx, client, pr, heads)))
			}
		}
		for _, review := range activity.Reviews {
			if window.Contains(review.OccurredAt) {
				add(prEntry(review, pullRequestHead(ctx, client, review, heads)))
			}
		}
	}
	return entries, pushErr
}

// fetchEvents returns the user's events since the given time, newest first.
// Private events are included when the client is authenticated as the user.
func fetchEvents(ctx context.Context, client *Client, username string, since time.Time) ([]*gh.Event, error) {
	var events []*gh.Event
	opts := &gh.ListOptions{PerPage: 100}
	for range maxEventPages {
		batch, response, err := client.REST.Activity.ListEventsPerformedByUser(ctx, username, false, opts)
		if err != nil {
			return nil, fmt.Errorf("could not list GitHub events: %w", err)
		}
		reachedOlder := false
		for _, event := range batch {
			if event.GetCreatedAt().Before(since) {
				reachedOlder = true
				continue
			}
			events = append(events, event)
		}
		if reachedOlder || response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}
	return events, nil
}

type pushedBranch struct {
	repo, branch, head string
}

// pushedCommits lists the commits the user authored in the window on every
// branch they pushed to since it started. Each branch is read from its latest
// pushed head, and a commit on several branches keeps the one pushed first.
func pushedCommits(ctx context.Context, client *Client, username string, events []*gh.Event, window collector.Window) ([]store.Entry, error) {
	var branches []pushedBranch
	known := map[string]bool{}
	for _, event := range events {
		if event.GetType() != "PushEvent" {
			continue
		}
		payload, err := event.ParsePayload()
		if err != nil {
			continue
		}
		push, ok := payload.(*gh.PushEvent)
		if !ok {
			continue
		}
		branch, ok := strings.CutPrefix(push.GetRef(), "refs/heads/")
		if !ok || push.GetHead() == "" {
			continue
		}
		key := event.GetRepo().GetName() + "\x00" + branch
		if !known[key] {
			known[key] = true
			branches = append(branches, pushedBranch{repo: event.GetRepo().GetName(), branch: branch, head: push.GetHead()})
		}
	}

	var entries []store.Entry
	var errs []error
	for i := len(branches) - 1; i >= 0; i-- {
		branch := branches[i]
		owner, name, ok := strings.Cut(branch.repo, "/")
		if !ok {
			continue
		}
		opts := &gh.CommitsListOptions{SHA: branch.head, Author: username, Since: window.From, ListOptions: gh.ListOptions{PerPage: 100}}
		for range maxCommitPages {
			commits, response, err := client.REST.Repositories.ListCommits(ctx, owner, name, opts)
			if err != nil {
				// A force-push or a deleted repository can leave a head that no
				// longer exists; that branch has nothing left to report.
				var apiErr *gh.ErrorResponse
				if errors.As(err, &apiErr) && apiErr.Response != nil && (apiErr.Response.StatusCode == http.StatusNotFound || apiErr.Response.StatusCode == http.StatusConflict || apiErr.Response.StatusCode == http.StatusUnprocessableEntity) {
					break
				}
				errs = append(errs, fmt.Errorf("could not list commits of %s on %s: %w", branch.repo, branch.branch, err))
				break
			}
			for _, commit := range commits {
				authored := commit.GetCommit().GetAuthor().GetDate().Time
				if len(commit.Parents) > 1 || commit.GetCommitter().GetLogin() == webFlowLogin || !window.Contains(authored) {
					continue
				}
				subject, body := splitMessage(commit.GetCommit().GetMessage())
				entries = append(entries, commitEntry(branch.repo, branch.branch, commit.GetSHA(), subject, body, commit.GetHTMLURL(), authored))
			}
			if response.NextPage == 0 {
				break
			}
			opts.Page = response.NextPage
		}
	}
	return entries, errors.Join(errs...)
}

type commentDay struct {
	repo   string
	number int
	day    string
}

// eventEntries maps comments and created branches from the event feed.
// Comments are grouped per issue or pull request and day.
func eventEntries(events []*gh.Event, window collector.Window) []store.Entry {
	var entries []store.Entry
	comments := map[commentDay]*store.Entry{}
	var order []commentDay
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		at := event.GetCreatedAt().Time
		if !window.Contains(at) {
			continue
		}
		repo := event.GetRepo().GetName()
		payload, err := event.ParsePayload()
		if err != nil {
			continue
		}
		var number int
		var title, url, noun string
		switch payload := payload.(type) {
		case *gh.CreateEvent:
			if payload.GetRefType() == "branch" {
				entries = append(entries, store.Entry{
					Project:     collector.ShortRepoName(repo),
					Description: "Created branch " + payload.GetRef(),
					Tags:        []string{"github", "branch"},
					Source:      fmt.Sprintf("github:branch-created:%s:%s", repo, payload.GetRef()),
					Activity: &store.Activity{
						Kind: store.KindBranchCreated, Collector: "github", OccurredAt: at,
						Repo: repo, Branch: payload.GetRef(), URL: fmt.Sprintf("https://github.com/%s/tree/%s", repo, payload.GetRef()),
					},
				})
			}
			continue
		case *gh.IssueCommentEvent:
			if payload.GetAction() != "created" {
				continue
			}
			number, title, url, noun = payload.GetIssue().GetNumber(), payload.GetIssue().GetTitle(), payload.GetIssue().GetHTMLURL(), "issue"
			if payload.GetIssue().IsPullRequest() {
				noun = "PR"
			}
		case *gh.PullRequestReviewCommentEvent:
			if payload.GetAction() != "created" {
				continue
			}
			number, title, url, noun = payload.GetPullRequest().GetNumber(), payload.GetPullRequest().GetTitle(), payload.GetPullRequest().GetHTMLURL(), "PR"
		default:
			continue
		}
		key := commentDay{repo: repo, number: number, day: window.Day(at)}
		if entry := comments[key]; entry != nil {
			entry.Activity.Count++
			ended := at
			entry.Activity.EndedAt = &ended
			continue
		}
		comments[key] = &store.Entry{
			Project:     collector.ShortRepoName(repo),
			Description: fmt.Sprintf("Commented on %s #%d: %s", noun, number, title),
			Tags:        []string{"github", "comment"},
			Source:      fmt.Sprintf("github:comment:%s#%d:%s", repo, number, key.day),
			Activity: &store.Activity{
				Kind: store.KindComment, Collector: "github", OccurredAt: at,
				Repo: repo, Number: number, URL: url, Count: 1,
			},
		}
		order = append(order, key)
	}
	for _, key := range order {
		entries = append(entries, *comments[key])
	}
	return entries
}

// pullRequestHead returns the head branch of a pull request, which carries the
// ticket key more reliably than its title. Lookups are cached in heads.
func pullRequestHead(ctx context.Context, client *Client, pr PRActivity, heads map[string]string) string {
	key := fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
	if head, ok := heads[key]; ok {
		return head
	}
	head := ""
	if owner, name, ok := strings.Cut(pr.Repo, "/"); ok {
		if pull, _, err := client.REST.PullRequests.Get(ctx, owner, name, pr.Number); err == nil {
			head = pull.GetHead().GetRef()
		}
	}
	heads[key] = head
	return head
}

func commitEntry(repo, branch, sha, subject, body, url string, authored time.Time) store.Entry {
	description := subject
	if body != "" {
		description += "\n\n" + collector.Truncate(body, maxBodyLength)
	}
	return store.Entry{
		Project:     collector.ShortRepoName(repo),
		Description: description,
		Tags:        []string{"github", "commit"},
		Source:      "commit:" + sha,
		Activity: &store.Activity{
			Kind: store.KindCommit, Collector: "github", OccurredAt: authored,
			Repo: repo, Branch: branch, SHA: sha, URL: url,
		},
	}
}

func prEntry(pr PRActivity, branch string) store.Entry {
	kind, tag := store.KindPR, "pr"
	source := fmt.Sprintf("github:pr:%s#%d:%s", pr.Repo, pr.Number, pr.Action)
	if pr.Action == "reviewed" {
		kind, tag = store.KindReview, "review"
		source = fmt.Sprintf("github:review:%s#%d:%s", pr.Repo, pr.Number, pr.OccurredAt.UTC().Format(time.RFC3339Nano))
	}
	action := strings.ToUpper(pr.Action[:1]) + pr.Action[1:]
	return store.Entry{
		Project:     collector.ShortRepoName(pr.Repo),
		Description: prDescription(fmt.Sprintf("%s PR #%d: %s", action, pr.Number, pr.Title), pr.Body),
		Tags:        []string{"github", tag},
		Source:      source,
		Activity: &store.Activity{
			Kind: kind, Collector: "github", OccurredAt: pr.OccurredAt,
			Repo: pr.Repo, Branch: branch, Number: pr.Number, Action: pr.Action, URL: pr.URL,
		},
	}
}

func prDescription(summary, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return summary
	}
	return summary + "\n\n" + collector.Truncate(body, maxBodyLength)
}

// utcDays lists the UTC dates the window touches, the granularity of GitHub's
// date search qualifiers.
func utcDays(window collector.Window) []time.Time {
	first := window.From.UTC()
	last := window.To.Add(-time.Nanosecond).UTC()
	day := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.UTC)
	var days []time.Time
	for !day.After(last) {
		days = append(days, day)
		day = day.AddDate(0, 0, 1)
	}
	return days
}
