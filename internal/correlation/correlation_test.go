package correlation

import (
	"testing"
	"time"

	"devlog/internal/database"
	"devlog/internal/domain"
)

func TestGroupsByProjectAndIdleGap(t *testing.T) {
	start := time.Date(2026, 7, 12, 9, 0, 0, 0, time.UTC)
	events := []domain.Event{{ID: "1", ProjectID: "a", Kind: "manual.entry", OccurredAt: start, Payload: database.EncodePayload(map[string]string{"description": "Reviewed API"})}, {ID: "2", ProjectID: "a", Kind: "git.snapshot", OccurredAt: start.Add(10 * time.Minute)}, {ID: "3", ProjectID: "a", Kind: "git.commit", OccurredAt: start.Add(2 * time.Hour), Payload: database.EncodePayload(map[string]string{"message": "Fixed sync"})}, {ID: "4", ProjectID: "b", Kind: "github.issue", OccurredAt: start.Add(2*time.Hour + time.Minute), Payload: database.EncodePayload(map[string]string{"title": "Plan collector"})}}
	activities := (Correlator{IdleGap: 45 * time.Minute}).Correlate(events)
	if len(activities) != 3 {
		t.Fatalf("got %d activities", len(activities))
	}
	if activities[0].Description != "Reviewed API" {
		t.Fatalf("description=%q", activities[0].Description)
	}
	if activities[1].Confidence != domain.ConfidenceHigh {
		t.Fatalf("confidence=%s", activities[1].Confidence)
	}
}
