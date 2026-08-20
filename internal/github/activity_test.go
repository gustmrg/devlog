package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v74/github"
)

func TestFetchActivityCollectsEveryCommitPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/commits":
			page := r.URL.Query().Get("page")
			if page == "" {
				w.Header().Set("Link", fmt.Sprintf(`<%s/search/commits?page=2>; rel="next"`, serverURL(r)))
				_, _ = w.Write([]byte(commitSearchPage("first")))
				return
			}
			_, _ = w.Write([]byte(commitSearchPage("second")))
		case "/search/issues":
			_, _ = w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
		case "/graphql":
			_, _ = w.Write([]byte(emptyReviewContributionPage()))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := gh.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL
	activity, err := FetchActivity(context.Background(), &Client{REST: client, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}, "octocat", time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.Commits) != 2 || activity.Commits[0].SHA != "first" || activity.Commits[1].SHA != "second" {
		t.Fatalf("commits = %+v, want both pages", activity.Commits)
	}
}

func TestFetchActivityRejectsIncompleteSearchResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/commits" {
			_, _ = w.Write([]byte(`{"total_count":1,"incomplete_results":true,"items":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := gh.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL
	_, err := FetchActivity(context.Background(), &Client{REST: client, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}, "octocat", time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error = %v, want incomplete-results failure", err)
	}
}

func TestFetchActivityRejectsCappedSearchResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/commits" {
			_, _ = w.Write([]byte(commitSearchPage("only-result")))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := gh.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	client.BaseURL = baseURL
	_, err := FetchActivity(context.Background(), &Client{REST: client, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}, "octocat", time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("error = %v, want capped-results failure", err)
	}
}

func TestFetchActivityUsesDateBoundedPaginatedReviewContributions(t *testing.T) {
	var graphqlRequests int
	var variables []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/commits", "/search/issues":
			_, _ = w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
		case "/graphql":
			graphqlRequests++
			var request struct {
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode GraphQL request: %v", err)
			}
			variables = append(variables, request.Variables)
			if graphqlRequests == 1 {
				_, _ = w.Write([]byte(reviewContributionPage("2026-08-08T10:30:00Z", true, "next")))
				return
			}
			_, _ = w.Write([]byte(reviewContributionPage("2026-08-08T18:45:00Z", false, "")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	rest := gh.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	rest.BaseURL = baseURL
	activity, err := FetchActivity(context.Background(), &Client{REST: rest, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}, "octocat", time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if graphqlRequests != 2 {
		t.Fatalf("GraphQL requests = %d, want 2", graphqlRequests)
	}
	if variables[0]["from"] != "2026-08-08T00:00:00Z" || variables[0]["to"] != "2026-08-09T00:00:00Z" {
		t.Fatalf("date variables = %+v", variables[0])
	}
	if variables[0]["after"] != nil || variables[1]["after"] != "next" {
		t.Fatalf("cursor variables = %+v", variables)
	}
	if len(activity.Reviews) != 2 {
		t.Fatalf("reviews = %+v, want both pages", activity.Reviews)
	}
	if got := activity.Reviews[0].OccurredAt.Format(time.RFC3339); got != "2026-08-08T10:30:00Z" {
		t.Fatalf("occurredAt = %s", got)
	}
}

func TestFetchActivityKeepsOpenedAndMergedEventsForSamePR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/commits":
			_, _ = w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
		case "/search/issues":
			_, _ = w.Write([]byte(`{"total_count":1,"incomplete_results":false,"items":[{"number":42,"title":"Ship it","repository_url":"https://api.github.com/repos/acme/widgets","html_url":"https://github.com/acme/widgets/pull/42"}]}`))
		case "/graphql":
			_, _ = w.Write([]byte(emptyReviewContributionPage()))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	rest := gh.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	rest.BaseURL = baseURL
	activity, err := FetchActivity(context.Background(), &Client{REST: rest, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}, "octocat", time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(activity.PRs) != 2 || activity.PRs[0].Action != "opened" || activity.PRs[1].Action != "merged" {
		t.Fatalf("pull requests = %+v, want opened and merged events", activity.PRs)
	}
}

func TestFetchActivityReportsGraphQLErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "API error", body: `{"errors":[{"message":"review access denied"}]}`, want: "review access denied"},
		{name: "malformed response", body: `{`, want: "invalid review response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/search/commits", "/search/issues":
					_, _ = w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
				case "/graphql":
					_, _ = w.Write([]byte(test.body))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			rest := gh.NewClient(server.Client())
			baseURL, _ := url.Parse(server.URL + "/")
			rest.BaseURL = baseURL
			_, err := FetchActivity(context.Background(), &Client{REST: rest, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}, "octocat", time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFetchActivityCancelsGraphQLRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/commits", "/search/issues":
			_, _ = w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
		case "/graphql":
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	rest := gh.NewClient(server.Client())
	baseURL, _ := url.Parse(server.URL + "/")
	rest.BaseURL = baseURL
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := FetchActivity(ctx, &Client{REST: rest, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}, "octocat", time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
		result <- err
	}()
	<-started
	cancel()
	err := <-result
	close(release)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
}

func commitSearchPage(sha string) string {
	return fmt.Sprintf(`{"total_count":2,"incomplete_results":false,"items":[{"sha":%q,"repository":{"full_name":"acme/widgets"},"commit":{"message":%q},"html_url":"https://github.com/acme/widgets/commit/%s"}]}`, sha, sha, sha)
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func reviewContributionPage(occurredAt string, hasNext bool, cursor string) string {
	return fmt.Sprintf(`{"data":{"user":{"contributionsCollection":{"pullRequestReviewContributions":{"nodes":[{"occurredAt":%q,"pullRequest":{"number":42,"title":"Ship it","url":"https://github.com/acme/widgets/pull/42","repository":{"nameWithOwner":"acme/widgets"}}}],"pageInfo":{"hasNextPage":%t,"endCursor":%q}}}}}}`, occurredAt, hasNext, cursor)
}

func emptyReviewContributionPage() string {
	return `{"data":{"user":{"contributionsCollection":{"pullRequestReviewContributions":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`
}
