// Package github fetches a user's GitHub activity (commits, pull requests,
// reviews) for a given date, across all repositories the token can see.
package github

import (
	"context"
	"net/http"

	gh "github.com/google/go-github/v74/github"
	"golang.org/x/oauth2"
)

const defaultGraphQLURL = "https://api.github.com/graphql"

// Client contains the authenticated transports used by GitHub's REST and
// GraphQL APIs.
type Client struct {
	REST       *gh.Client
	HTTP       *http.Client
	GraphQLURL string
}

// NewClient returns an authenticated GitHub API client.
func NewClient(ctx context.Context, token string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	return &Client{REST: gh.NewClient(tc), HTTP: tc, GraphQLURL: defaultGraphQLURL}
}
