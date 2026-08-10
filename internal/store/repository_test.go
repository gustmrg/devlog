package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepositoryMigratesLegacyData(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := filepath.Join(configRoot, "data")
	legacyEntries := filepath.Join(configRoot, "entries")
	legacySummaries := filepath.Join(configRoot, "summaries")
	if err := os.MkdirAll(legacyEntries, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacySummaries, 0755); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 8, 8, 14, 30, 0, 0, time.UTC)
	legacy := DailyLog{Date: "2026-08-08", Entries: []Entry{
		{Id: "manual", Project: "devlog", Description: "manual", CreatedAt: created},
		{Id: "random-old-id", Project: "devlog", Description: "imported", Source: "github:commit:abc", CreatedAt: created},
	}}
	if err := SaveDailyLog(filepath.Join(legacyEntries, "2026-08-08.json"), legacy); err != nil {
		t.Fatal(err)
	}
	summary := []byte("---\ndate: 2026-08-08\nstyle: concise\nprojects: devlog\n---\n- done\n")
	if err := os.WriteFile(filepath.Join(legacySummaries, "2026-08-08.md"), summary, 0644); err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(dataRoot, configRoot)
	result, err := repo.EnsureLayout()
	if err != nil {
		t.Fatal(err)
	}
	if result.MigratedEntries != 2 || result.MigratedSummaries != 1 || result.BackupPath == "" {
		t.Fatalf("unexpected migration result: %+v", result)
	}
	entries, err := repo.Entries(date)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[1].Source == "github:commit:abc" && entries[1].Id != EntryID(entries[1].Source) {
		t.Fatalf("imported entry ID is not deterministic: %s", entries[1].Id)
	}
	if _, err := os.Stat(filepath.Join(result.BackupPath, "entries", "2026-08-08.json")); err != nil {
		t.Fatalf("legacy backup missing: %v", err)
	}
	second, err := repo.EnsureLayout()
	if err != nil || !second.AlreadyCurrent {
		t.Fatalf("second migration was not idempotent: %+v, %v", second, err)
	}
}

func TestRepositoryDeduplicatesExternalSources(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "data"), root)
	if _, err := repo.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	first := Entry{Id: EntryID("github:commit:abc"), Source: "github:commit:abc", CreatedAt: now}
	second := Entry{Id: "different", Source: first.Source, CreatedAt: now.Add(time.Second)}
	added, err := repo.AddEntries(date, []Entry{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 {
		t.Fatalf("added %d entries, want 1", len(added))
	}
}

func TestSummaryMetadataRoundTrip(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "data"), root)
	if _, err := repo.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	summary := Summary{Date: date, Style: "concise", Content: "- done", AIGenerated: true}
	if err := repo.SaveSummary(summary); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.LoadSummary(date)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GeneratedAt.IsZero() || loaded.DeviceID == "" || !loaded.AIGenerated {
		t.Fatalf("summary metadata was not persisted: %+v", loaded)
	}
}

func TestUnsafeDataRootDetection(t *testing.T) {
	configRoot := filepath.Join(t.TempDir(), ".devlog")
	if !unsafeDataRoot(configRoot, configRoot) {
		t.Fatal("configuration root was accepted as the data root")
	}
	if !unsafeDataRoot(filepath.Dir(configRoot), configRoot) {
		t.Fatal("a data root containing configuration was accepted")
	}
	if unsafeDataRoot(filepath.Join(configRoot, "data"), configRoot) {
		t.Fatal("dedicated data directory was rejected")
	}
}

func TestEntryIDsCannotEscapeDataDirectory(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "data"), root)
	if _, err := repo.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	added, err := repo.AddEntries(date, []Entry{{Id: "../../outside", CreatedAt: time.Now().UTC()}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].Id == "../../outside" {
		t.Fatalf("unsafe entry ID was not normalized: %+v", added)
	}
	if _, err := os.Stat(filepath.Join(root, "outside.json")); !os.IsNotExist(err) {
		t.Fatalf("entry escaped data directory: %v", err)
	}
}

func TestRepositoryLockBlocksUntilReleased(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "data"), root)
	firstUnlock, err := repo.Lock()
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan func(), 1)
	errs := make(chan error, 1)
	go func() {
		unlock, err := repo.Lock()
		if err != nil {
			errs <- err
			return
		}
		acquired <- unlock
	}()

	select {
	case unlock := <-acquired:
		unlock()
		firstUnlock()
		t.Fatal("second lock was acquired before the first was released")
	case err := <-errs:
		firstUnlock()
		t.Fatal(err)
	case <-time.After(100 * time.Millisecond):
	}

	firstUnlock()
	select {
	case unlock := <-acquired:
		unlock()
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("second lock did not acquire after release")
	}
}
