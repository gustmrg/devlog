// Package agentsession turns the sessions of coding agents into entries.
package agentsession

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"devlog/internal/collector"
	"devlog/internal/collector/gitlocal"
	"devlog/internal/store"
)

// Day is a session's activity on one calendar day.
type Day struct {
	First, Last time.Time
	// Prompts counts the messages the person sent.
	Prompts int
	Dir     string
	Branch  string
}

// Add extends the day to include t.
func (d *Day) Add(t time.Time) {
	if d.First.IsZero() || t.Before(d.First) {
		d.First = t
	}
	if t.After(d.Last) {
		d.Last = t
	}
}

// Repository is the repository a session ran in.
type Repository = gitlocal.Repository

// Repos identifies the repositories sessions ran in, once per directory.
type Repos struct {
	Git   string
	cache map[string]gitlocal.Repository
}

// Identify returns the repository containing dir, or one named after the
// directory when it is not in a repository.
func (r *Repos) Identify(ctx context.Context, dir string) gitlocal.Repository {
	if r.cache == nil {
		r.cache = map[string]gitlocal.Repository{}
	}
	if repo, ok := r.cache[dir]; ok {
		return repo
	}
	repo, err := gitlocal.Identify(ctx, r.Git, dir)
	if err != nil {
		repo = gitlocal.Repository{Dir: dir}
	}
	r.cache[dir] = repo
	return repo
}

// Entry builds the entry of one session's activity on day. Tool names the
// agent, and the title falls back to "<Tool name> session".
func Entry(tool, toolName, id, day, title string, activity Day, repo gitlocal.Repository) store.Entry {
	if title == "" {
		title = toolName + " session"
	}
	project := collector.ShortRepoName(repo.Name)
	if project == "" && activity.Dir != "" {
		project = filepath.Base(activity.Dir)
	}
	entry := store.Entry{
		Project:     project,
		Description: title,
		Tags:        []string{tool, "session"},
		Source:      fmt.Sprintf("%s:session:%s:%s", tool, id, day),
		Activity: &store.Activity{
			Kind:       store.KindSession,
			Collector:  tool,
			OccurredAt: activity.First,
			Repo:       repo.Name,
			Branch:     activity.Branch,
			Directory:  activity.Dir,
			Count:      activity.Prompts,
		},
	}
	if activity.Last.After(activity.First) {
		last := activity.Last
		entry.Activity.EndedAt = &last
	}
	return entry
}
