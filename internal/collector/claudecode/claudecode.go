// Package claudecode collects Claude Code sessions from the transcripts
// Claude Code keeps on this device. Only times, places, and titles are read;
// prompts and replies are not stored.
package claudecode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"devlog/internal/collector"
	"devlog/internal/collector/agentsession"
	"devlog/internal/store"
)

// Collector reads the transcripts under Dir.
type Collector struct {
	// Dir holds Claude Code's per-project transcripts. When empty it is
	// $CLAUDE_CONFIG_DIR/projects or ~/.claude/projects.
	Dir string
	Git string
}

func (c *Collector) Name() string { return "claude-code" }

// DefaultDir is where Claude Code keeps transcripts on this device.
func DefaultDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "projects"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

type session struct {
	title string
	days  map[string]*agentsession.Day
}

func (c *Collector) Collect(ctx context.Context, request collector.Request) ([]store.Entry, error) {
	dir := c.Dir
	if dir == "" {
		var err error
		if dir, err = DefaultDir(); err != nil {
			return nil, err
		}
	} else if rest, ok := strings.CutPrefix(dir, "~"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, rest)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, collector.Skip("no Claude Code transcripts found in %s", dir)
	}
	window := request.Window
	sessions := map[string]*session{}
	var order []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return nil
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		// A transcript not written since the window started has nothing in it.
		if info, err := entry.Info(); err != nil || info.ModTime().Before(window.From) {
			return nil
		}
		return readTranscript(path, window, func(id string) *session {
			if sessions[id] == nil {
				sessions[id] = &session{days: map[string]*agentsession.Day{}}
				order = append(order, id)
			}
			return sessions[id]
		})
	})
	if err != nil {
		return nil, fmt.Errorf("could not read Claude Code transcripts in %s: %w", dir, err)
	}

	repos := &agentsession.Repos{Git: c.Git}
	var entries []store.Entry
	for _, id := range order {
		s := sessions[id]
		for day, activity := range s.days {
			var repo agentsession.Repository
			if activity.Dir != "" {
				repo = repos.Identify(ctx, activity.Dir)
			}
			entries = append(entries, agentsession.Entry("claude-code", "Claude Code", id, day, s.title, *activity, repo))
		}
	}
	return entries, nil
}

type transcriptLine struct {
	Type        string          `json:"type"`
	Timestamp   time.Time       `json:"timestamp"`
	SessionID   string          `json:"sessionId"`
	Cwd         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	AITitle     string          `json:"aiTitle"`
	CustomTitle string          `json:"customTitle"`
	Summary     string          `json:"summary"`
	Message     json.RawMessage `json:"message"`
}

func readTranscript(path string, window collector.Window, sessionFor func(id string) *session) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	fallbackID := filepath.Base(path[:len(path)-len(filepath.Ext(path))])
	reader := bufio.NewReaderSize(file, 1<<20)
	customTitled := map[string]bool{}
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var record transcriptLine
			if json.Unmarshal(line, &record) == nil {
				id := record.SessionID
				if id == "" {
					id = fallbackID
				}
				s := sessionFor(id)
				switch record.Type {
				case "custom-title":
					if record.CustomTitle != "" {
						s.title, customTitled[id] = record.CustomTitle, true
					}
				case "ai-title":
					if record.AITitle != "" && !customTitled[id] {
						s.title = record.AITitle
					}
				case "summary":
					if record.Summary != "" && s.title == "" {
						s.title = record.Summary
					}
				case "user", "assistant":
					if record.Timestamp.IsZero() || !window.Contains(record.Timestamp) {
						break
					}
					day := window.Day(record.Timestamp)
					activity := s.days[day]
					if activity == nil {
						activity = &agentsession.Day{}
						s.days[day] = activity
					}
					activity.Add(record.Timestamp)
					if record.Cwd != "" {
						activity.Dir = record.Cwd
					}
					if record.GitBranch != "" && record.GitBranch != "HEAD" {
						activity.Branch = record.GitBranch
					}
					if record.Type == "user" && !record.IsMeta && !record.IsSidechain && isPrompt(record.Message) {
						activity.Prompts++
					}
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// isPrompt reports whether a user message was typed by the person rather
// than returned by a tool.
func isPrompt(message json.RawMessage) bool {
	var body struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(message, &body) != nil || len(body.Content) == 0 {
		return false
	}
	if body.Content[0] == '"' {
		return true
	}
	var parts []struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(body.Content, &parts) == nil && len(parts) > 0 && parts[0].Type == "text"
}
