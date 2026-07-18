package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"devlog/internal/database"
	"devlog/internal/domain"
	gh "github.com/google/go-github/v74/github"
	"github.com/google/uuid"
)

type Collector struct {
	Token, Owner, Repo, ProjectID, Actor string
	Client                               *gh.Client
}

func (c *Collector) Type() string { return "github" }
func (c *Collector) Validate() error {
	if c.Token == "" {
		return fmt.Errorf("GitHub token is required")
	}
	if c.Owner == "" || c.Repo == "" {
		return fmt.Errorf("GitHub owner and repo are required")
	}
	return nil
}
func (c *Collector) Collect(ctx context.Context, cursor string) ([]domain.Event, string, error) {
	if err := c.Validate(); err != nil {
		return nil, cursor, err
	}
	client := c.Client
	if client == nil {
		client = gh.NewClient(nil).WithAuthToken(c.Token)
	}
	actor := c.Actor
	if actor == "" {
		user, _, err := client.Users.Get(ctx, "")
		if err != nil {
			return nil, cursor, err
		}
		actor = user.GetLogin()
	}
	since := time.Now().Add(-24 * time.Hour)
	if cursor != "" {
		if parsed, err := time.Parse(time.RFC3339, cursor); err == nil {
			since = parsed.Add(-5 * time.Minute)
		}
	}
	var events []domain.Event
	latest := since
	commitOpts := &gh.CommitsListOptions{Since: since, Author: actor, ListOptions: gh.ListOptions{PerPage: 100}}
	for {
		commits, response, err := client.Repositories.ListCommits(ctx, c.Owner, c.Repo, commitOpts)
		if err != nil {
			return nil, cursor, err
		}
		for _, commit := range commits {
			at := commit.GetCommit().GetAuthor().GetDate().Time
			if at.After(latest) {
				latest = at
			}
			message := strings.Split(commit.GetCommit().GetMessage(), "\n")[0]
			events = append(events, c.event("github.commit", commit.GetSHA(), at, map[string]any{"message": message, "sha": commit.GetSHA(), "url": commit.GetHTMLURL()}))
		}
		if response.NextPage == 0 {
			break
		}
		commitOpts.Page = response.NextPage
	}
	issueOpts := &gh.IssueListByRepoOptions{State: "all", Since: since, ListOptions: gh.ListOptions{PerPage: 100}}
	var issues []*gh.Issue
	for {
		page, response, err := client.Issues.ListByRepo(ctx, c.Owner, c.Repo, issueOpts)
		if err != nil {
			return nil, cursor, err
		}
		issues = append(issues, page...)
		if response.NextPage == 0 {
			break
		}
		issueOpts.ListOptions.Page = response.NextPage
	}
	for _, issue := range issues {
		at := issue.GetUpdatedAt().Time
		if at.After(latest) {
			latest = at
		}
		kind := "github.issue"
		if issue.IsPullRequest() {
			kind = "github.pull_request"
		}
		if strings.EqualFold(issue.GetUser().GetLogin(), actor) {
			events = append(events, c.event(kind, fmt.Sprintf("%d:%s", issue.GetNumber(), at.Format(time.RFC3339Nano)), at, map[string]any{"number": issue.GetNumber(), "title": issue.GetTitle(), "state": issue.GetState(), "url": issue.GetHTMLURL()}))
		}
		if issue.IsPullRequest() {
			reviewOpts := &gh.ListOptions{PerPage: 100}
			for {
				reviews, response, err := client.PullRequests.ListReviews(ctx, c.Owner, c.Repo, issue.GetNumber(), reviewOpts)
				if err != nil {
					return nil, cursor, err
				}
				for _, review := range reviews {
					at := review.GetSubmittedAt().Time
					if at.Before(since) || !strings.EqualFold(review.GetUser().GetLogin(), actor) {
						continue
					}
					if at.After(latest) {
						latest = at
					}
					events = append(events, c.event("github.review", fmt.Sprint(review.GetID()), at, map[string]any{"number": issue.GetNumber(), "state": review.GetState(), "url": review.GetHTMLURL()}))
				}
				if response.NextPage == 0 {
					break
				}
				reviewOpts.Page = response.NextPage
			}
		}
	}
	commentOpts := &gh.IssueListCommentsOptions{Since: &since, ListOptions: gh.ListOptions{PerPage: 100}}
	for {
		comments, response, err := client.Issues.ListComments(ctx, c.Owner, c.Repo, 0, commentOpts)
		if err != nil {
			return nil, cursor, err
		}
		for _, comment := range comments {
			if !strings.EqualFold(comment.GetUser().GetLogin(), actor) {
				continue
			}
			at := comment.GetUpdatedAt().Time
			if at.After(latest) {
				latest = at
			}
			events = append(events, c.event("github.comment", fmt.Sprintf("%d:%s", comment.GetID(), at.Format(time.RFC3339Nano)), at, map[string]any{"url": comment.GetHTMLURL(), "author": comment.GetUser().GetLogin()}))
		}
		if response.NextPage == 0 {
			break
		}
		commentOpts.Page = response.NextPage
	}
	if latest.Before(time.Now().Add(-time.Minute)) {
		latest = time.Now().UTC()
	}
	return events, latest.UTC().Format(time.RFC3339), nil
}
func (c *Collector) event(kind, external string, at time.Time, payload any) domain.Event {
	fingerprint := fmt.Sprintf("github:%s/%s:%s:%s", c.Owner, c.Repo, kind, external)
	sum := sha256.Sum256([]byte(fingerprint))
	return domain.Event{ID: uuid.NewString(), SourceType: "github", SourceInstanceID: c.Owner + "/" + c.Repo, ExternalID: external, ProjectID: c.ProjectID, Kind: kind, OccurredAt: at.UTC(), ObservedAt: time.Now().UTC(), Payload: database.EncodePayload(payload), Fingerprint: hex.EncodeToString(sum[:])}
}

func ParseRemote(remote string) (owner, repo string, ok bool) {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	parts := strings.Split(remote, "/")
	if len(parts) < 3 {
		return "", "", false
	}
	host := parts[len(parts)-3]
	if !strings.Contains(host, "github.com") {
		return "", "", false
	}
	return parts[len(parts)-2], parts[len(parts)-1], true
}
