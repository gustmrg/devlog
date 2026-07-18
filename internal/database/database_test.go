package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"devlog/internal/domain"
)

func TestEventsAreDeduplicatedAndBackedUp(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.UpsertProject(ctx, domain.Project{ID: "devlog", Name: "DevLog", Enabled: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	event := domain.Event{ID: "event-1", SourceType: "git", SourceInstanceID: "repo", ExternalID: "head", ProjectID: "devlog", Kind: "git.commit", OccurredAt: now, ObservedAt: now, Fingerprint: "fingerprint"}
	if n, err := db.InsertEvents(ctx, []domain.Event{event, event}); err != nil || n != 1 {
		t.Fatalf("inserted=%d err=%v", n, err)
	}
	events, err := db.EventsForDay(ctx, now.Format("2006-01-02"), time.UTC)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err := db.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(backup); err != nil || info.Size() == 0 {
		t.Fatalf("invalid backup: %v", err)
	}
}

func TestOutboxAndDeviceRevocation(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	event := domain.Event{ID: "event-1", SourceType: "manual", SourceInstanceID: "cli", ExternalID: "1", Kind: "manual.entry", OccurredAt: now, ObservedAt: now, Fingerprint: "manual:1"}
	if _, err := db.QueueEvents(ctx, []domain.Event{event}); err != nil {
		t.Fatal(err)
	}
	pending, err := db.PendingEvents(ctx, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%d err=%v", len(pending), err)
	}
	if pending[0].Sequence != 1 {
		t.Fatalf("sequence=%d", pending[0].Sequence)
	}
	if err := db.AckEvents(ctx, []string{"event-1"}); err != nil {
		t.Fatal(err)
	}
	pending, _ = db.PendingEvents(ctx, 10)
	if len(pending) != 0 {
		t.Fatal("event was not acknowledged")
	}
	if err := db.CreateDevice(ctx, "device", "laptop", "hash"); err != nil {
		t.Fatal(err)
	}
	if id, err := db.AuthenticateDevice(ctx, "hash"); err != nil || id != "device" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	if err := db.RevokeDevice(ctx, "device"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AuthenticateDevice(ctx, "hash"); err == nil {
		t.Fatal("revoked token authenticated")
	}
}

func TestChangeLogCacheAndJobHistory(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devlog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := db.CreateActivity(ctx, "2026-07-12", domain.Activity{ID: "activity", Description: "Work", StartedAt: now, EndedAt: now, Status: domain.ActivityConfirmed, Confidence: domain.ConfidenceHigh, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	changes, err := db.ChangesAfter(ctx, 0, 10)
	if err != nil || len(changes) != 1 {
		t.Fatalf("changes=%v err=%v", changes, err)
	}
	if err := db.CacheTimeline(ctx, "2026-07-12", []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	cached, err := db.CachedTimeline(ctx, "2026-07-12")
	if err != nil || len(cached) == 0 {
		t.Fatalf("cache=%s err=%v", cached, err)
	}
	jobID, err := db.StartJob(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishJob(ctx, jobID, nil); err != nil {
		t.Fatal(err)
	}
	jobs, err := db.JobRuns(ctx, 10)
	if err != nil || len(jobs) != 1 || jobs[0].Status != "completed" {
		t.Fatalf("jobs=%v err=%v", jobs, err)
	}
}
