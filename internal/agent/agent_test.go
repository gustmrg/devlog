package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"devlog/internal/config"
	"devlog/internal/database"
	"devlog/internal/domain"
	"devlog/internal/syncapi"
)

func TestSyncPullsTimelineIntoOfflineCache(t *testing.T) {
	ctx := context.Background()
	remote, err := database.Open(filepath.Join(t.TempDir(), "remote.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	token := "token"
	if err := remote.CreateDevice(ctx, "device", "test", syncapi.HashToken(token)); err != nil {
		t.Fatal(err)
	}
	date := "2026-07-12"
	at := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	if err := remote.CreateActivity(ctx, date, domain.Activity{ID: "activity", Description: "Synced activity", StartedAt: at, EndedAt: at, Status: domain.ActivityConfirmed, Confidence: domain.ConfidenceHigh, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	handler := syncapi.Handler{DB: remote}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sync/changes", handler.Changes)
	mux.HandleFunc("GET /api/v1/timeline", handler.Timeline)
	server := httptest.NewServer(mux)
	defer server.Close()
	local, err := database.Open(filepath.Join(t.TempDir(), "local.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	a := Agent{Config: config.Config{Server: config.ServerConfig{URL: server.URL, AllowInsecure: true}}, DB: local, Credentials: syncapi.Credentials{DeviceID: "device", Token: token}}
	if err := a.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	cached, err := local.CachedTimeline(ctx, date)
	if err != nil || len(cached) == 0 {
		t.Fatalf("cached=%s err=%v", cached, err)
	}
	cursor, err := local.SyncCursor(ctx)
	if err != nil || cursor == 0 {
		t.Fatalf("cursor=%d err=%v", cursor, err)
	}
}
