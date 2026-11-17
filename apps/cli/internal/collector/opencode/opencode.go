// Package opencode collects opencode sessions through the opencode CLI, whose
// JSON output is stable while its storage changes between versions. Only
// times, places, and titles are kept; prompts and replies are not stored.
package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"devlog/internal/collector"
	"devlog/internal/collector/agentsession"
	"devlog/internal/collector/gitlocal"
	"devlog/internal/store"
)

const (
	commandTimeout = time.Minute
	listLimit      = "100"
)

// Collector lists the sessions of the repositories under Roots: opencode
// lists sessions per project, so it needs to know where to look.
type Collector struct {
	Roots   []string
	Command string
	Git     string
	// run executes the opencode CLI in a directory; tests replace it.
	run func(ctx context.Context, dir string, args ...string) ([]byte, error)
}

func (c *Collector) Name() string { return "opencode" }

type listedSession struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Created   int64  `json:"created"`
	Updated   int64  `json:"updated"`
	Directory string `json:"directory"`
}

type exportedSession struct {
	Messages []struct {
		Type string `json:"type"`
		Time struct {
			Created   int64 `json:"created"`
			Completed int64 `json:"completed"`
		} `json:"time"`
	} `json:"messages"`
}

func (c *Collector) Collect(ctx context.Context, request collector.Request) ([]store.Entry, error) {
	if len(c.Roots) == 0 {
		return nil, collector.Skip("opencode lists sessions per project; run devlog config set collect.git.roots <dir>[,<dir>...]")
	}
	run := c.run
	if run == nil {
		command := c.Command
		if command == "" {
			command = "opencode"
		}
		path, err := exec.LookPath(command)
		if err != nil {
			return nil, collector.Skip("opencode is not installed")
		}
		run = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
			return runCLI(ctx, path, dir, args...)
		}
	}
	dirs, err := gitlocal.Discover(c.Roots, 0)
	if err != nil {
		return nil, err
	}

	window := request.Window
	repos := &agentsession.Repos{Git: c.Git}
	seen := map[string]bool{}
	var entries []store.Entry
	var errs []error
	for _, dir := range dirs {
		output, err := run(ctx, dir, "session", "list", "--format", "json", "-n", listLimit, "--standalone")
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", dir, err))
			continue
		}
		var sessions []listedSession
		if err := json.Unmarshal(output, &sessions); err != nil {
			errs = append(errs, fmt.Errorf("%s: opencode returned an invalid session list: %w", dir, err))
			continue
		}
		for _, session := range sessions {
			created, updated := time.UnixMilli(session.Created), time.UnixMilli(session.Updated)
			if seen[session.ID] || updated.Before(window.From) || !created.Before(window.To) {
				continue
			}
			seen[session.ID] = true
			sessionDir := session.Directory
			if sessionDir == "" {
				sessionDir = dir
			}
			days := sessionDays(ctx, run, dir, session, window)
			repo := repos.Identify(ctx, sessionDir)
			for day, activity := range days {
				activity.Dir = sessionDir
				activity.Branch = gitlocal.BranchAt(ctx, c.Git, sessionDir, activity.First)
				entries = append(entries, agentsession.Entry("opencode", "opencode", session.ID, day, session.Title, *activity, repo))
			}
		}
	}
	return entries, errors.Join(errs...)
}

// sessionDays splits a session's messages by day. When the export fails it
// falls back to the session's creation and last update.
func sessionDays(ctx context.Context, run func(context.Context, string, ...string) ([]byte, error), dir string, session listedSession, window collector.Window) map[string]*agentsession.Day {
	days := map[string]*agentsession.Day{}
	add := func(t time.Time, prompt bool) {
		if !window.Contains(t) {
			return
		}
		day := window.Day(t)
		if days[day] == nil {
			days[day] = &agentsession.Day{}
		}
		days[day].Add(t)
		if prompt {
			days[day].Prompts++
		}
	}
	var exported exportedSession
	output, err := run(ctx, dir, "session", "export", session.ID, "--sanitize", "--standalone")
	if err == nil && json.Unmarshal(output, &exported) == nil && len(exported.Messages) > 0 {
		for _, message := range exported.Messages {
			if message.Time.Created > 0 {
				add(time.UnixMilli(message.Time.Created), message.Type == "user")
			}
			if message.Time.Completed > 0 {
				add(time.UnixMilli(message.Time.Completed), false)
			}
		}
		return days
	}
	add(time.UnixMilli(session.Created), false)
	add(time.UnixMilli(session.Updated), false)
	return days
}

func runCLI(ctx context.Context, path, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("opencode %s failed: %s", strings.Join(args[:2], " "), message)
	}
	return output, nil
}
