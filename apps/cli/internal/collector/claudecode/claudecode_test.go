package claudecode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devlog/internal/collector"
)

func TestCollectGroupsSessionsByDayWithoutPrompts(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	project := filepath.Join(dir, "-work-widgets")
	if err := os.MkdirAll(filepath.Join(project, "s1", "subagents"), 0755); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	quotedCwd, _ := json.Marshal(cwd)
	lines := []string{
		`{"type":"user","timestamp":"2026-08-17T12:00:00Z","sessionId":"s1","cwd":` + string(quotedCwd) + `,"gitBranch":"feature/PROJ-7","message":{"role":"user","content":"secret prompt text"}}`,
		`{"type":"assistant","timestamp":"2026-08-17T12:05:00Z","sessionId":"s1","cwd":` + string(quotedCwd) + `,"gitBranch":"feature/PROJ-7","message":{"role":"assistant","content":[{"type":"text","text":"reply"}]}}`,
		`{"type":"user","timestamp":"2026-08-17T12:06:00Z","sessionId":"s1","cwd":` + string(quotedCwd) + `,"gitBranch":"feature/PROJ-7","message":{"role":"user","content":[{"type":"tool_result","content":"out"}]}}`,
		`{"type":"user","timestamp":"2026-08-17T12:07:00Z","sessionId":"s1","isMeta":true,"message":{"role":"user","content":"<command>"}}`,
		`{"type":"ai-title","aiTitle":"Widget refactor","sessionId":"s1"}`,
		`{"type":"user","timestamp":"2026-08-18T11:00:00Z","sessionId":"s1","cwd":` + string(quotedCwd) + `,"gitBranch":"feature/PROJ-7","message":{"role":"user","content":[{"type":"text","text":"next day"}]}}`,
		`not json`,
	}
	if err := os.WriteFile(filepath.Join(project, "s1.jsonl"), []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
	subagent := `{"type":"assistant","timestamp":"2026-08-17T15:30:00Z","sessionId":"s1","isSidechain":true,"cwd":` + string(quotedCwd) + `,"message":{"content":"sub"}}` + "\n"
	if err := os.WriteFile(filepath.Join(project, "s1", "subagents", "agent-1.jsonl"), []byte(subagent), 0644); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(project, "old.jsonl")
	if err := os.WriteFile(old, []byte(`{"type":"user","timestamp":"2026-08-17T13:00:00Z","sessionId":"old","message":{"content":"x"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	stale := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(old, stale, stale); err != nil {
		t.Fatal(err)
	}

	window := collector.DayWindow(time.Date(2026, 8, 17, 0, 0, 0, 0, loc), loc)
	entries, err := (&Collector{Dir: dir}).Collect(context.Background(), collector.Request{Window: window})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want one session day", entries)
	}
	entry := entries[0]
	if entry.Source != "claude-code:session:s1:2026-08-17" || entry.Description != "Widget refactor" {
		t.Errorf("entry = %q %q", entry.Source, entry.Description)
	}
	activity := entry.Activity
	if activity.Count != 1 || activity.Branch != "feature/PROJ-7" || activity.Directory != cwd || entry.Project != filepath.Base(cwd) {
		t.Errorf("activity = %+v, project %q", activity, entry.Project)
	}
	if !activity.OccurredAt.Equal(time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)) || !activity.EndedAt.Equal(time.Date(2026, 8, 17, 15, 30, 0, 0, time.UTC)) {
		t.Errorf("window = %v to %v", activity.OccurredAt, activity.EndedAt)
	}
	if strings.Contains(entry.Description, "secret") {
		t.Error("a prompt leaked into the entry")
	}
}

func TestCollectSkipsWithoutTranscripts(t *testing.T) {
	_, err := (&Collector{Dir: filepath.Join(t.TempDir(), "missing")}).Collect(context.Background(), collector.Request{})
	if _, ok := err.(*collector.SkippedError); !ok {
		t.Fatalf("error = %v, want a skip", err)
	}
}
