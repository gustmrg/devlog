package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"devlog/internal/agent"
	"devlog/internal/store"
)

func TestLegacyMigrationIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	entriesDir := filepath.Join(home, ".devlog", "entries")
	if err := os.MkdirAll(entriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := store.DailyLog{Date: "2026-07-12", Entries: []store.Entry{{Id: "legacy-id", Project: "devlog", Description: "Implemented migration", CreatedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}}}
	if err := store.SaveDailyLog(filepath.Join(entriesDir, "2026-07-12.json"), legacy); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		cmd := newMigrateLegacyCmd()
		cmd.SetArgs([]string{"--yes"})
		cmd.SetOut(new(bytes.Buffer))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	db, err := agent.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	events, err := db.EventsForDay(t.Context(), "2026-07-12", time.UTC)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
}
