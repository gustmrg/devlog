// Package github fetches a user's GitHub activity (commits, pull requests,
// reviews) for a given date, across all repositories the token can see.
package github

import (
	"context"

	gh "github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

// NewClient returns an authenticated GitHub API client.
func NewClient(ctx context.Context, token string) *gh.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	return gh.NewClient(tc)
}
