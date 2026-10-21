package opencode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devlog/internal/collector"
)

func TestCollectListsSessionsPerRepositoryAndSplitsDays(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo := filepath.Join(root, "widgets")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	ms := func(value string) int64 {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed.UnixMilli()
	}
	var calls []string
	run := func(_ context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args[:3], " "))
		switch {
		case args[1] == "list":
			return []byte(fmt.Sprintf(`[
				{"id":"ses_a","title":"Widget API","created":%d,"updated":%d,"directory":%q},
				{"id":"ses_old","title":"Old","created":%d,"updated":%d,"directory":%q},
				{"id":"ses_b","title":"No export","created":%d,"updated":%d,"directory":%q}
			]`, ms("2026-08-17T12:00:00Z"), ms("2026-08-18T13:00:00Z"), dir,
				ms("2026-08-10T12:00:00Z"), ms("2026-08-10T13:00:00Z"), dir,
				ms("2026-08-17T18:00:00Z"), ms("2026-08-17T19:00:00Z"), dir)), nil
		case args[1] == "export" && args[2] == "ses_a":
			return []byte(fmt.Sprintf(`{"info":{},"messages":[
				{"type":"user","time":{"created":%d}},
				{"type":"assistant","time":{"created":%d,"completed":%d}},
				{"type":"user","time":{"created":%d}}
			]}`, ms("2026-08-17T12:00:00Z"), ms("2026-08-17T12:01:00Z"), ms("2026-08-17T12:30:00Z"), ms("2026-08-18T13:00:00Z"))), nil
		default:
			return nil, errors.New("export failed")
		}
	}
	c := &Collector{Roots: []string{root}, run: run}
	window := collector.DayWindow(time.Date(2026, 8, 17, 0, 0, 0, 0, loc), loc)
	entries, err := c.Collect(context.Background(), collector.Request{Window: window})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want ses_a and ses_b on 2026-08-17", entries)
	}
	byID := map[string]int{}
	for i, entry := range entries {
		byID[entry.Source] = i
	}
	a := entries[byID["opencode:session:ses_a:2026-08-17"]]
	if a.Description != "Widget API" || a.Activity.Count != 1 || !a.Activity.EndedAt.Equal(time.Date(2026, 8, 17, 12, 30, 0, 0, time.UTC)) || a.Project != "widgets" {
		t.Errorf("ses_a = %+v %+v", a, a.Activity)
	}
	b, ok := byID["opencode:session:ses_b:2026-08-17"]
	if !ok || !entries[b].Activity.EndedAt.Equal(time.Date(2026, 8, 17, 19, 0, 0, 0, time.UTC)) {
		t.Errorf("ses_b should fall back to its creation and update times: %+v", entries)
	}
	if calls[0] != "session list --format" {
		t.Errorf("first call = %q", calls[0])
	}
}

func TestCollectSkipsWithoutRoots(t *testing.T) {
	_, err := (&Collector{}).Collect(context.Background(), collector.Request{})
	if _, ok := err.(*collector.SkippedError); !ok {
		t.Fatalf("error = %v, want a skip", err)
	}
}
