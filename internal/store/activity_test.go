package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "data"), root)
	if _, err := repo.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	return repo
}

func localZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func commitEntry(occurredAt time.Time, branch string) Entry {
	return Entry{
		Project:     "widgets",
		Description: "feat: add widgets",
		Source:      "commit:abc",
		Activity: &Activity{
			Kind:       KindCommit,
			Collector:  "git",
			OccurredAt: occurredAt,
			Repo:       "acme/widgets",
			Branch:     branch,
			SHA:        "abc",
		},
	}
}

func TestUpsertEntriesStoresEntriesOnTheirLocalDay(t *testing.T) {
	repo := newTestRepository(t)
	loc := localZone(t)
	// 01:30 UTC is still the previous evening in Los Angeles.
	occurred := time.Date(2026, 8, 18, 1, 30, 0, 0, time.UTC)
	result, err := repo.UpsertEntries([]Entry{commitEntry(occurred, "main")}, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 1 {
		t.Fatalf("added = %d, want 1", len(result.Added))
	}
	entries, err := repo.Entries(time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Id != EntryID("commit:abc") {
		t.Fatalf("entries on 2026-08-17 = %+v, want the commit", entries)
	}
	if got := entries[0].Activity.OccurredAt.Format(time.RFC3339); got != "2026-08-17T18:30:00-07:00" {
		t.Errorf("occurredAt = %s, want it stored in Los Angeles time", got)
	}
}

func TestUpsertEntriesFillsMissingFieldsAndConverges(t *testing.T) {
	repo := newTestRepository(t)
	loc := localZone(t)
	occurred := time.Date(2026, 8, 17, 10, 0, 0, 0, loc)

	first := commitEntry(occurred, "")
	first.Activity.Collector = "github"
	first.Activity.URL = "https://github.com/acme/widgets/commit/abc"
	if _, err := repo.UpsertEntries([]Entry{first}, loc); err != nil {
		t.Fatal(err)
	}

	second := commitEntry(occurred, "feature/PROJ-1-widgets")
	second.Description = "a different description"
	second.Activity.Tickets = []string{"PROJ-1"}
	result, err := repo.UpsertEntries([]Entry{second}, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) != 1 {
		t.Fatalf("updated = %d, want 1", len(result.Updated))
	}
	merged := result.Updated[0]
	if merged.Description != "feat: add widgets" {
		t.Errorf("description = %q, want the stored one", merged.Description)
	}
	if merged.Activity.Branch != "feature/PROJ-1-widgets" || merged.Activity.URL == "" || merged.Activity.Collector != "github" {
		t.Errorf("activity = %+v, want stored fields kept and missing ones filled", merged.Activity)
	}
	if len(merged.Activity.Tickets) != 1 || merged.Activity.Tickets[0] != "PROJ-1" {
		t.Errorf("tickets = %v, want [PROJ-1]", merged.Activity.Tickets)
	}

	again, err := repo.UpsertEntries([]Entry{second, first}, loc)
	if err != nil {
		t.Fatal(err)
	}
	if again.Unchanged != 2 || len(again.Added)+len(again.Updated) != 0 {
		t.Fatalf("repeated upsert = %+v, want everything unchanged", again)
	}
}

func TestUpsertEntriesReplacesSnapshots(t *testing.T) {
	repo := newTestRepository(t)
	loc := localZone(t)
	started := time.Date(2026, 8, 17, 9, 0, 0, 0, loc)
	session := func(end time.Time, title string) Entry {
		return Entry{Project: "widgets", Description: title, Source: "claude-code:session:s1:2026-08-17", Activity: &Activity{
			Kind: KindSession, Collector: "claude-code", OccurredAt: started, EndedAt: &end, Count: 1,
		}}
	}
	if _, err := repo.UpsertEntries([]Entry{session(started.Add(time.Hour), "")}, loc); err != nil {
		t.Fatal(err)
	}
	result, err := repo.UpsertEntries([]Entry{session(started.Add(3*time.Hour), "Widget refactor")}, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) != 1 {
		t.Fatalf("updated = %d, want 1", len(result.Updated))
	}
	got := result.Updated[0]
	if got.Description != "Widget refactor" || !got.Activity.EndedAt.Equal(started.Add(3*time.Hour)) {
		t.Errorf("snapshot = %q until %v, want the newer values", got.Description, got.Activity.EndedAt)
	}
}

func TestUpsertEntriesMergesIntoLegacyCommitSource(t *testing.T) {
	repo := newTestRepository(t)
	loc := localZone(t)
	day := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	legacy := Entry{Project: "widgets", Description: "Polished description", Source: "github:commit:abc", CreatedAt: time.Now()}
	if _, err := repo.AddEntries(day, []Entry{legacy}); err != nil {
		t.Fatal(err)
	}
	result, err := repo.UpsertEntries([]Entry{commitEntry(time.Date(2026, 8, 17, 10, 0, 0, 0, loc), "main")}, loc)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 0 || len(result.Updated) != 1 {
		t.Fatalf("result = %+v, want the legacy entry updated", result)
	}
	entries, err := repo.Entries(day)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Description != "Polished description" || entries[0].Activity == nil {
		t.Fatalf("entries = %+v, want one legacy entry with activity", entries)
	}
}

func TestUpsertEntriesKeepsDeletedEntriesDeleted(t *testing.T) {
	repo := newTestRepository(t)
	loc := localZone(t)
	entry := commitEntry(time.Date(2026, 8, 17, 10, 0, 0, 0, loc), "main")
	result, err := repo.UpsertEntries([]Entry{entry}, loc)
	if err != nil {
		t.Fatal(err)
	}
	deleted := result.Added[0]
	now := time.Now()
	deleted.DeletedAt = &now
	data, _ := json.Marshal(deleted)
	path := filepath.Join(repo.Root, "entries", "2026-08-17", deleted.Id+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	entry.Activity.Branch = "feature/x"
	again, err := repo.UpsertEntries([]Entry{entry}, loc)
	if err != nil {
		t.Fatal(err)
	}
	if again.Unchanged != 1 {
		t.Fatalf("result = %+v, want the deleted entry left alone", again)
	}
}

func TestUpsertEntriesRequiresSourceAndActivity(t *testing.T) {
	repo := newTestRepository(t)
	if _, err := repo.UpsertEntries([]Entry{{Description: "manual"}}, time.UTC); err == nil {
		t.Fatal("UpsertEntries accepted an entry without source and activity")
	}
}

func TestRecordCollectorRunsKeepsLastSuccessAfterFailure(t *testing.T) {
	repo := newTestRepository(t)
	success := time.Date(2026, 8, 17, 12, 15, 0, 0, time.UTC)
	if err := repo.RecordCollectorRuns("laptop", map[string]CollectorStatus{
		"git": {LastRunAt: success, LastSuccessAt: success, CoveredFrom: success.Add(-12 * time.Hour), CoveredTo: success, Entries: 3},
	}); err != nil {
		t.Fatal(err)
	}
	failure := success.Add(6 * time.Hour)
	if err := repo.RecordCollectorRuns("laptop", map[string]CollectorStatus{
		"git":         {LastRunAt: failure, Error: "boom"},
		"claude-code": {LastRunAt: failure, LastSuccessAt: failure, Entries: 1},
	}); err != nil {
		t.Fatal(err)
	}
	statuses, err := repo.DeviceStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Name != "laptop" {
		t.Fatalf("statuses = %+v, want one laptop status", statuses)
	}
	git := statuses[0].Collectors["git"]
	if git.Error != "boom" || !git.LastSuccessAt.Equal(success) || !git.CoveredTo.Equal(success) || !git.LastRunAt.Equal(failure) {
		t.Errorf("git status = %+v, want the failure with the earlier success kept", git)
	}
	if _, ok := statuses[0].Collectors["claude-code"]; !ok {
		t.Error("claude-code status was not recorded")
	}
}
