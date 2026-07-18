package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	gh "github.com/google/go-github/v74/github"
)

func TestParseRemote(t *testing.T) {
	for _, remote := range []string{"github.com/gustmrg/devlog", "https://github.com/gustmrg/devlog.git"} {
		owner, repo, ok := ParseRemote(remote)
		if !ok || owner != "gustmrg" || repo != "devlog" {
			t.Fatalf("remote %q => %s/%s %t", remote, owner, repo, ok)
		}
	}
	if _, _, ok := ParseRemote("gitlab.com/gustmrg/devlog"); ok {
		t.Fatal("accepted non-GitHub remote")
	}
}

func TestCollectsCommitIssueAndComment(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"login":"dev"}`) })
	mux.HandleFunc("/repos/o/r/commits", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"sha":"abc","html_url":"https://example/commit","commit":{"message":"Implement sync","author":{"date":"2026-07-12T12:00:00Z"}}}]`)
	})
	mux.HandleFunc("/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"number":1,"title":"Track work","state":"open","updated_at":"2026-07-12T12:10:00Z","html_url":"https://example/issue","user":{"login":"dev"}}]`)
	})
	mux.HandleFunc("/repos/o/r/issues/comments", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":2,"updated_at":"2026-07-12T12:20:00Z","html_url":"https://example/comment","user":{"login":"dev"}}]`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := gh.NewClient(nil)
	base, _ := url.Parse(server.URL + "/")
	client.BaseURL = base
	collector := Collector{Token: "token", Owner: "o", Repo: "r", ProjectID: "project", Client: client}
	events, cursor, err := collector.Collect(context.Background(), "2026-07-12T11:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		var kinds []string
		for _, e := range events {
			kinds = append(kinds, e.Kind)
		}
		t.Fatalf("events=%v", kinds)
	}
	if cursor == "" || strings.HasPrefix(cursor, "2026-07-12T11:00:00") {
		t.Fatalf("cursor=%s", cursor)
	}
}
