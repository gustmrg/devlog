package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"devlog/internal/config"
	"devlog/internal/domain"
)

func TestHealthLoginTimelineAndReview(t *testing.T) {
	cfg := config.Default()
	srv, err := New(cfg, t.TempDir(), "PAIR", "password", "http://example.test")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()
	resp, err := http.Get(httpServer.URL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("health=%v err=%v", resp.Status, err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	form := url.Values{"password": {"password"}}
	resp, err = client.PostForm(httpServer.URL+"/login", form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(jar.Cookies(mustURL(t, httpServer.URL))) == 0 {
		t.Fatal("missing session cookie")
	}
	date := time.Now().In(time.FixedZone("local", -3*3600)).Format("2006-01-02")
	if err := srv.AddActivity(context.Background(), date, "Met with the team", ""); err != nil {
		t.Fatal(err)
	}
	resp, err = client.Get(httpServer.URL + "/days/" + date)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if !strings.Contains(buf.String(), "Met with the team") {
		t.Fatalf("timeline did not render activity: %s", buf.String())
	}
}
func TestSummaryRevision(t *testing.T) {
	cfg := config.Default()
	srv, err := New(cfg, t.TempDir(), "PAIR", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	date := "2026-07-12"
	if err := srv.AddActivity(context.Background(), date, "Implemented sync", "devlog"); err == nil {
		t.Fatal("expected missing project foreign key")
	}
	if err := srv.DB.UpsertProject(context.Background(), domain.Project{ID: "devlog", Name: "DevLog", Enabled: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := srv.AddActivity(context.Background(), date, "Implemented sync", "devlog"); err != nil {
		t.Fatal(err)
	}
	if err := srv.GenerateSummary(context.Background(), date); err != nil {
		t.Fatal(err)
	}
	summary, err := srv.DB.LatestSummary(context.Background(), date)
	if err != nil || summary.Revision != 1 || !strings.Contains(summary.Content, "Implemented sync") {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
}

func TestConfirmedActivityIsNotRecreatedByCorrelation(t *testing.T) {
	cfg := config.Default()
	srv, err := New(cfg, t.TempDir(), "PAIR", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ctx := context.Background()
	date := "2026-07-12"
	if err := srv.DB.UpsertProject(ctx, domain.Project{ID: "devlog", Name: "DevLog", Enabled: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	event := domain.Event{ID: "event", SourceType: "git", SourceInstanceID: "devlog", ExternalID: "commit", ProjectID: "devlog", Kind: "git.commit", OccurredAt: at, ObservedAt: at, Fingerprint: "commit"}
	if _, err := srv.DB.InsertEvents(ctx, []domain.Event{event}); err != nil {
		t.Fatal(err)
	}
	if err := srv.Correlate(ctx, date); err != nil {
		t.Fatal(err)
	}
	activities, err := srv.DB.ActivitiesForDay(ctx, date)
	if err != nil || len(activities) != 1 {
		t.Fatalf("activities=%d err=%v", len(activities), err)
	}
	if err := srv.DB.SetActivityStatus(ctx, activities[0].ID, domain.ActivityConfirmed); err != nil {
		t.Fatal(err)
	}
	if err := srv.Correlate(ctx, date); err != nil {
		t.Fatal(err)
	}
	activities, err = srv.DB.ActivitiesForDay(ctx, date)
	if err != nil || len(activities) != 1 {
		t.Fatalf("confirmed activity was duplicated: %d err=%v", len(activities), err)
	}
}
func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
