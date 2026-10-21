package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"devlog/internal/collector"
	"devlog/internal/store"

	gh "github.com/google/go-github/v74/github"
)

func TestPREntryUsesDistinctSourcesForPRActions(t *testing.T) {
	opened := prEntry(PRActivity{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "opened"}, "")
	merged := prEntry(PRActivity{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "merged"}, "")
	if opened.Source == merged.Source {
		t.Fatalf("opened and merged sources collide: %q", opened.Source)
	}
}

func TestPREntryUsesReviewOccurrenceInSource(t *testing.T) {
	first := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	a := prEntry(PRActivity{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "reviewed", OccurredAt: first}, "")
	b := prEntry(PRActivity{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "reviewed", OccurredAt: first.Add(24 * time.Hour)}, "")
	if a.Source == b.Source {
		t.Fatalf("review sources collide: %q", a.Source)
	}
	if a.Activity.Kind != store.KindReview {
		t.Errorf("review kind = %q", a.Activity.Kind)
	}
}

func TestPREntryIncludesPRBodyInDescription(t *testing.T) {
	for _, test := range []struct {
		pr   PRActivity
		want string
	}{
		{PRActivity{Repo: "acme/widgets", Number: 42, Title: "Ship it", Body: "  Adds the new widget flow.  ", Action: "opened"}, "Opened PR #42: Ship it\n\nAdds the new widget flow."},
		{PRActivity{Repo: "acme/widgets", Number: 43, Title: "No details", Action: "merged"}, "Merged PR #43: No details"},
		{PRActivity{Repo: "acme/widgets", Number: 44, Title: "Review it", Body: "Review context", Action: "reviewed"}, "Reviewed PR #44: Review it\n\nReview context"},
	} {
		if got := prEntry(test.pr, "").Description; got != test.want {
			t.Errorf("description = %q, want %q", got, test.want)
		}
	}
}

func TestCollectorReadsPushesSearchesAndEvents(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	window := collector.DayWindow(time.Date(2026, 8, 17, 0, 0, 0, 0, loc), loc)
	var commitQueries []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/octocat/events":
			_, _ = w.Write([]byte(`[
				{"type":"IssueCommentEvent","created_at":"2026-08-17T19:00:00Z","repo":{"name":"acme/widgets"},"payload":{"action":"created","issue":{"number":42,"title":"Ship it","html_url":"https://github.com/acme/widgets/pull/42","pull_request":{"url":"x"}}}},
				{"type":"PullRequestReviewCommentEvent","created_at":"2026-08-17T18:00:00Z","repo":{"name":"acme/widgets"},"payload":{"action":"created","pull_request":{"number":42,"title":"Ship it","html_url":"https://github.com/acme/widgets/pull/42"}}},
				{"type":"PushEvent","created_at":"2026-08-17T17:00:00Z","repo":{"name":"acme/widgets"},"payload":{"ref":"refs/heads/feature/PROJ-7-widgets","head":"head2"}},
				{"type":"PushEvent","created_at":"2026-08-17T15:00:00Z","repo":{"name":"acme/widgets"},"payload":{"ref":"refs/heads/feature/PROJ-7-widgets","head":"head1"}},
				{"type":"CreateEvent","created_at":"2026-08-17T12:00:00Z","repo":{"name":"acme/widgets"},"payload":{"ref":"feature/PROJ-7-widgets","ref_type":"branch"}},
				{"type":"PushEvent","created_at":"2026-08-10T12:00:00Z","repo":{"name":"acme/old"},"payload":{"ref":"refs/heads/main","head":"old"}}
			]`))
		case "/repos/acme/widgets/commits":
			commitQueries = append(commitQueries, r.URL.Query())
			_, _ = w.Write([]byte(`[
				{"sha":"c1","html_url":"https://github.com/acme/widgets/commit/c1","commit":{"message":"feat: widgets\n\nWhy widgets.","author":{"date":"2026-08-17T13:00:00Z"}},"parents":[{"sha":"p"}]},
				{"sha":"m1","commit":{"message":"Merge branch 'develop'","author":{"date":"2026-08-17T14:00:00Z"}},"parents":[{"sha":"a"},{"sha":"b"}]},
				{"sha":"y1","commit":{"message":"feat: yesterday","author":{"date":"2026-08-16T13:00:00Z"}},"parents":[{"sha":"p"}]}
			]`))
		case "/search/commits":
			_, _ = w.Write([]byte(`{"total_count":2,"incomplete_results":false,"items":[
				{"sha":"c1","repository":{"full_name":"acme/widgets"},"commit":{"message":"feat: widgets","author":{"date":"2026-08-17T13:00:00Z"}},"parents":[{"sha":"p"}]},
				{"sha":"s1","repository":{"full_name":"acme/widgets"},"commit":{"message":"Ship it (#42)","author":{"date":"2026-08-17T20:00:00Z"}},"committer":{"login":"web-flow"},"parents":[{"sha":"p"}]}
			]}`))
		case "/search/issues":
			if strings.Contains(r.URL.Query().Get("q"), "created:") {
				_, _ = w.Write([]byte(`{"total_count":1,"incomplete_results":false,"items":[{"number":42,"title":"Feature/proj 7 widgets","repository_url":"https://api.github.com/repos/acme/widgets","html_url":"https://github.com/acme/widgets/pull/42","created_at":"2026-08-17T16:00:00Z"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"total_count":0,"incomplete_results":false,"items":[]}`))
		case "/repos/acme/widgets/pulls/42":
			_, _ = w.Write([]byte(`{"number":42,"head":{"ref":"feature/PROJ-7-widgets"}}`))
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
	client := &Client{REST: rest, HTTP: server.Client(), GraphQLURL: server.URL + "/graphql"}
	c := &Collector{Connect: func(context.Context) (*Client, string, error) { return client, "octocat", nil }}
	entries, err := c.Collect(context.Background(), collector.Request{Window: window})
	if err != nil {
		t.Fatal(err)
	}

	if len(commitQueries) != 1 || commitQueries[0].Get("sha") != "head2" || commitQueries[0].Get("author") != "octocat" {
		t.Fatalf("commit queries = %v, want one from the latest head", commitQueries)
	}
	bySource := map[string]store.Entry{}
	for _, entry := range entries {
		bySource[entry.Source] = entry
	}
	commit, ok := bySource["commit:c1"]
	if !ok || commit.Activity.Branch != "feature/PROJ-7-widgets" || commit.Description != "feat: widgets\n\nWhy widgets." {
		t.Fatalf("commit c1 = %+v, want it from the push with its branch", commit)
	}
	for _, source := range []string{"commit:m1", "commit:y1", "commit:s1"} {
		if _, ok := bySource[source]; ok {
			t.Errorf("%s was collected; merges, other days, and GitHub-made commits are skipped", source)
		}
	}
	pr, ok := bySource["github:pr:acme/widgets#42:opened"]
	if !ok || pr.Activity.Branch != "feature/PROJ-7-widgets" || !pr.Activity.OccurredAt.Equal(time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("opened PR = %+v, want its head branch and creation time", pr)
	}
	comment, ok := bySource["github:comment:acme/widgets#42:2026-08-17"]
	if !ok || comment.Activity.Count != 2 || comment.Description != "Commented on PR #42: Ship it" {
		t.Fatalf("comment = %+v, want both comments grouped", comment)
	}
	if _, ok := bySource["github:branch-created:acme/widgets:feature/PROJ-7-widgets"]; !ok {
		t.Error("created branch was not collected")
	}
	if len(entries) != 4 {
		t.Errorf("entries = %d, want 4: %v", len(entries), entries)
	}
}

func TestUTCDaysCoversTheLocalWindow(t *testing.T) {
	loc := time.FixedZone("UTC-3", -3*3600)
	days := utcDays(collector.DayWindow(time.Date(2026, 8, 17, 0, 0, 0, 0, loc), loc))
	got := fmt.Sprint(days[0].Format("2006-01-02"), " ", days[len(days)-1].Format("2006-01-02"), " ", len(days))
	if got != "2026-08-17 2026-08-18 2" {
		t.Fatalf("utcDays = %s", got)
	}
}
