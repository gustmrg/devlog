package cmd

import (
	"testing"
	"time"

	ghsync "devlog/internal/github"
)

func TestMapActivityUsesDistinctSourcesForPRActions(t *testing.T) {
	entries := mapActivity(ghsync.Activity{PRs: []ghsync.PRActivity{
		{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "opened"},
		{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "merged"},
	}})

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Source == entries[1].Source {
		t.Fatalf("opened and merged sources collide: %q", entries[0].Source)
	}
}

func TestMapActivityUsesReviewOccurrenceInSource(t *testing.T) {
	first := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	second := first.Add(24 * time.Hour)
	entries := mapActivity(ghsync.Activity{Reviews: []ghsync.PRActivity{
		{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "reviewed", OccurredAt: first},
		{Repo: "acme/widgets", Number: 42, Title: "Ship it", Action: "reviewed", OccurredAt: second},
	}})

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].Source == entries[1].Source {
		t.Fatalf("review sources collide: %q", entries[0].Source)
	}
}
