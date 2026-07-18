package correlation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"devlog/internal/domain"
	"github.com/google/uuid"
)

type Correlator struct{ IdleGap time.Duration }

func (c Correlator) Correlate(events []domain.Event) []domain.Activity {
	if c.IdleGap == 0 {
		c.IdleGap = 45 * time.Minute
	}
	sort.Slice(events, func(i, j int) bool { return events[i].OccurredAt.Before(events[j].OccurredAt) })
	var groups [][]domain.Event
	for _, e := range events {
		if len(groups) == 0 {
			groups = append(groups, []domain.Event{e})
			continue
		}
		last := groups[len(groups)-1]
		prev := last[len(last)-1]
		if e.ProjectID != prev.ProjectID || e.OccurredAt.Sub(prev.OccurredAt) > c.IdleGap {
			groups = append(groups, []domain.Event{e})
		} else {
			groups[len(groups)-1] = append(last, e)
		}
	}
	activities := make([]domain.Activity, 0, len(groups))
	now := time.Now().UTC()
	for _, g := range groups {
		project := g[0].ProjectID
		description := describe(g)
		confidence := domain.ConfidenceLow
		if len(g) >= 2 {
			confidence = domain.ConfidenceMedium
		}
		for _, e := range g {
			if strings.Contains(e.Kind, "commit") || strings.Contains(e.Kind, "pull_request") {
				confidence = domain.ConfidenceHigh
			}
		}
		a := domain.Activity{ID: uuid.NewString(), ProjectID: project, Description: description, StartedAt: g[0].OccurredAt, EndedAt: g[len(g)-1].OccurredAt, Status: domain.ActivityDraft, Confidence: confidence, UpdatedAt: now}
		for _, e := range g {
			a.Evidence = append(a.Evidence, domain.Evidence{EventID: e.ID, Label: e.Kind})
		}
		activities = append(activities, a)
	}
	return activities
}

func describe(events []domain.Event) string {
	e := events[len(events)-1]
	for i := len(events) - 1; i >= 0; i-- {
		var p map[string]any
		_ = json.Unmarshal(events[i].Payload, &p)
		if message, _ := p["message"].(string); message != "" {
			return message
		}
		if title, _ := p["title"].(string); title != "" {
			return title
		}
		if description, _ := p["description"].(string); description != "" {
			return description
		}
	}
	project := e.ProjectID
	if project == "" {
		project = "unclassified"
	}
	return fmt.Sprintf("Worked on %s (%d captured signal%s)", project, len(events), map[bool]string{true: "", false: "s"}[len(events) == 1])
}
