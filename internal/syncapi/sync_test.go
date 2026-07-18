package syncapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"devlog/internal/database"
	"devlog/internal/domain"
)

func TestPairPushAndRetry(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := Handler{DB: db, PairingCode: "PAIR"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/devices/pair", handler.Pair)
	mux.HandleFunc("POST /api/v1/sync/events", handler.Push)
	mux.HandleFunc("GET /api/v1/sync/changes", handler.Changes)
	mux.HandleFunc("GET /api/v1/timeline", handler.Timeline)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := Client{BaseURL: server.URL}
	paired, err := client.Pair(context.Background(), "PAIR", "laptop")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	event := domain.Event{ID: "one", SourceType: "git", SourceInstanceID: "repo", ExternalID: "head", ProjectID: "project", Kind: "git.commit", OccurredAt: now, ObservedAt: now, Fingerprint: "git:one"}
	client.Token = paired.Token
	first, err := client.Push(context.Background(), []domain.Event{event})
	if err != nil || first.Accepted != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	retry, err := client.Push(context.Background(), []domain.Event{event})
	if err != nil || retry.Accepted != 0 || len(retry.Acknowledged) != 1 {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	date := now.Format("2006-01-02")
	if err := db.CreateActivity(context.Background(), date, domain.Activity{ID: "activity", Description: "Unified work", StartedAt: now, EndedAt: now, Status: domain.ActivityConfirmed, Confidence: domain.ConfidenceHigh, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	changes, err := client.Changes(context.Background(), 0)
	if err != nil || len(changes.Changes) == 0 {
		t.Fatalf("changes=%+v err=%v", changes, err)
	}
	timeline, err := client.Timeline(context.Background(), date)
	if err != nil || len(timeline.Activities) != 1 {
		t.Fatalf("timeline=%+v err=%v", timeline, err)
	}
}
