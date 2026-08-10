package gitremote

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"devlog/internal/store"
)

func TestTwoMachineSynchronization(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	cmd := exec.Command("git", "init", "--bare", bare)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %s", output)
	}

	repoA := newTestRepository(t, filepath.Join(root, "machine-a"))
	repoB := newTestRepository(t, filepath.Join(root, "machine-b"))
	managerA := New(repoA, bare, "main")
	managerB := New(repoB, bare, "main")
	if err := managerA.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := managerB.Init(ctx); err != nil {
		t.Fatal(err)
	}

	date := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	_, err := repoA.AddEntries(date, []store.Entry{{Id: "from-a", Project: "a", Description: "A", CreatedAt: now, UpdatedAt: now}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repoB.AddEntries(date, []store.Entry{{Id: "from-b", Project: "b", Description: "B", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := managerA.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := managerB.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := managerA.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	assertEntryCount(t, repoA, date, 2)
	assertEntryCount(t, repoB, date, 2)

	source := "github:commit:same"
	older := now.Add(2 * time.Second)
	newer := older.Add(time.Second)
	_, _ = repoA.AddEntries(date, []store.Entry{{Id: store.EntryID(source), Source: source, Description: "older", CreatedAt: older, UpdatedAt: older}})
	_, _ = repoB.AddEntries(date, []store.Entry{{Id: store.EntryID(source), Source: source, Description: "newer", CreatedAt: newer, UpdatedAt: newer}})
	if err := managerA.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := managerB.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := repoB.Entries(date)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, entry := range entries {
		if entry.Source == source {
			seen++
			if entry.Description != "newer" {
				t.Fatalf("conflict winner = %q, want newer", entry.Description)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("saw duplicated external source %d times", seen)
	}

	if err := managerA.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	olderSummary := store.Summary{Date: date, Style: "concise", Content: "from A", GeneratedAt: now.Add(4 * time.Second), DeviceID: "machine-a"}
	newerSummary := store.Summary{Date: date, Style: "concise", Content: "from B", GeneratedAt: now.Add(5 * time.Second), DeviceID: "machine-b"}
	if err := repoA.SaveSummary(olderSummary); err != nil {
		t.Fatal(err)
	}
	if err := repoB.SaveSummary(newerSummary); err != nil {
		t.Fatal(err)
	}
	if err := managerA.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := managerB.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := repoB.LoadSummary(date)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != "from B" {
		t.Fatalf("summary conflict winner = %q, want from B", loaded.Content)
	}
	conflicts, err := os.ReadDir(filepath.Join(repoB.Root, "summaries", "conflicts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("preserved summary conflicts = %d, want 1", len(conflicts))
	}
}

func TestSynchronizationPreservesSameSourceOnDifferentDates(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ctx := context.Background()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	if output, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %s", output)
	}

	repoA := newTestRepository(t, filepath.Join(root, "machine-a"))
	repoB := newTestRepository(t, filepath.Join(root, "machine-b"))
	managerA := New(repoA, bare, "main")
	managerB := New(repoB, bare, "main")
	if err := managerA.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := managerB.Init(ctx); err != nil {
		t.Fatal(err)
	}

	firstDate := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	secondDate := firstDate.AddDate(0, 0, 1)
	legacySource := "github:review:acme/widgets#42"
	_, err := repoA.AddEntries(firstDate, []store.Entry{{Source: legacySource, Description: "first review", CreatedAt: firstDate}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repoB.AddEntries(secondDate, []store.Entry{{Source: legacySource, Description: "second review", CreatedAt: secondDate}})
	if err != nil {
		t.Fatal(err)
	}
	if err := managerA.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := managerB.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	assertEntryCount(t, repoB, firstDate, 1)
	assertEntryCount(t, repoB, secondDate, 1)
}

func newTestRepository(t *testing.T, root string) *store.Repository {
	t.Helper()
	configRoot := filepath.Join(root, "config")
	repo := store.NewRepository(filepath.Join(root, "data"), configRoot)
	if _, err := repo.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	return repo
}

func assertEntryCount(t *testing.T, repo *store.Repository, date time.Time, wanted int) {
	t.Helper()
	entries, err := repo.Entries(date)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != wanted {
		t.Fatalf("entry count = %d, want %d", len(entries), wanted)
	}
}
