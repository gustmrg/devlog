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

func TestMapActivityIncludesPRBodyInDescription(t *testing.T) {
	entries := mapActivity(ghsync.Activity{
		PRs: []ghsync.PRActivity{
			{Repo: "acme/widgets", Number: 42, Title: "Ship it", Body: "  Adds the new widget flow.  ", Action: "opened"},
			{Repo: "acme/widgets", Number: 43, Title: "No details", Action: "merged"},
		},
		Reviews: []ghsync.PRActivity{
			{Repo: "acme/widgets", Number: 44, Title: "Review it", Body: "Review context", Action: "reviewed", OccurredAt: time.Now()},
		},
	})

	wants := []string{
		"Opened PR #42: Ship it\n\nAdds the new widget flow.",
		"Merged PR #43: No details",
		"Reviewed PR #44: Review it\n\nReview context",
	}
	if len(entries) != len(wants) {
		t.Fatalf("entries = %d, want %d", len(entries), len(wants))
	}
	for i, want := range wants {
		if entries[i].Description != want {
			t.Errorf("entry %d description = %q, want %q", i, entries[i].Description, want)
		}
	}
}
