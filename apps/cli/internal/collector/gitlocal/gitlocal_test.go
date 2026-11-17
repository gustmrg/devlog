package gitlocal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"devlog/internal/collector"
	"devlog/internal/store"
)

const me = "me@example.com"

func gitAt(t *testing.T, dir string, at time.Time, email string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	stamp := at.Format(time.RFC3339)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL="+email, "GIT_AUTHOR_DATE="+stamp,
		"GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL="+email, "GIT_COMMITTER_DATE="+stamp,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %s", args, output)
	}
}

func isolateGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func writeFile(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestCollectReadsCommitsBranchesAndUncommittedWork(t *testing.T) {
	isolateGit(t)
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	at := func(hour, minute int) time.Time { return time.Date(2026, 8, 17, hour, minute, 0, 0, loc) }

	root := t.TempDir()
	dir := filepath.Join(root, "work", "widgets")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	gitAt(t, dir, at(8, 0), me, "init", "-b", "main")
	gitAt(t, dir, at(8, 0), me, "remote", "add", "origin", "git@github.com:acme/widgets.git")
	writeFile(t, filepath.Join(dir, "a.txt"), "a\n", at(9, 0))
	gitAt(t, dir, at(9, 0), me, "add", ".")
	gitAt(t, dir, at(9, 0), me, "commit", "-m", "chore: start")
	gitAt(t, dir, at(10, 0), me, "checkout", "-b", "feature/PROJ-7-widgets")
	writeFile(t, filepath.Join(dir, "b.txt"), "b\n", at(10, 30))
	gitAt(t, dir, at(10, 30), me, "add", ".")
	gitAt(t, dir, at(10, 30), me, "commit", "-m", "feat: add widgets", "-m", "Explains the widgets.")
	writeFile(t, filepath.Join(dir, "c.txt"), "c\n", at(11, 0))
	gitAt(t, dir, at(11, 0), "someone@example.com", "add", ".")
	gitAt(t, dir, at(11, 0), "someone@example.com", "commit", "-m", "feat: someone else's work")
	writeFile(t, filepath.Join(dir, "a.txt"), "a\nmore\n", at(14, 0))
	writeFile(t, filepath.Join(dir, "new.txt"), "new\n", at(14, 5))

	c := &Collector{Roots: []string{root}, AuthorEmails: []string{me}, now: func() time.Time { return at(15, 0) }}
	window := collector.DayWindow(at(0, 0), loc)
	entries, err := c.Collect(context.Background(), collector.Request{Window: window, DeviceID: "device-1"})
	if err != nil {
		t.Fatal(err)
	}

	commits := map[string]store.Entry{}
	var branches []store.Entry
	for _, entry := range entries {
		switch entry.Activity.Kind {
		case store.KindCommit:
			commits[entry.Description] = entry
		case store.KindBranchActivity:
			branches = append(branches, entry)
		}
	}
	if len(commits) != 2 {
		t.Fatalf("commits = %v, want only the two authored by me", commits)
	}
	feature, ok := commits["feat: add widgets\n\nExplains the widgets."]
	if !ok {
		t.Fatalf("commits = %v, want the feature commit with its body", commits)
	}
	if feature.Activity.Branch != "feature/PROJ-7-widgets" || feature.Activity.Repo != "acme/widgets" || feature.Project != "widgets" {
		t.Errorf("feature commit activity = %+v", feature.Activity)
	}
	if !feature.Activity.OccurredAt.Equal(at(10, 30)) || feature.Source != "commit:"+feature.Activity.SHA {
		t.Errorf("feature commit = %+v", feature)
	}

	if len(branches) != 1 {
		t.Fatalf("branch activity = %+v, want only the feature branch", branches)
	}
	branch := branches[0]
	if branch.Activity.Branch != "feature/PROJ-7-widgets" || !branch.Activity.OccurredAt.Equal(at(10, 0)) || !branch.Activity.EndedAt.Equal(at(14, 5)) {
		t.Errorf("branch activity = %+v from %v to %v", branch.Activity, branch.Activity.OccurredAt, branch.Activity.EndedAt)
	}
	if branch.Source != "git:branch:device-1:acme/widgets:feature/PROJ-7-widgets:2026-08-17" {
		t.Errorf("branch source = %q", branch.Source)
	}
	want := store.Changes{Files: 1, Insertions: 1, Untracked: 1, Unpushed: 3}
	if branch.Activity.Changes == nil || *branch.Activity.Changes != want {
		t.Errorf("changes = %+v, want %+v", branch.Activity.Changes, want)
	}

	results := collector.Run(context.Background(), []collector.Collector{c}, window, collector.Options{
		DeviceName: "laptop", Tickets: regexp.MustCompile(collector.DefaultTicketPattern),
	})
	for _, entry := range results[0].Entries {
		if entry.Activity.Branch == "feature/PROJ-7-widgets" && (len(entry.Activity.Tickets) != 1 || entry.Activity.Tickets[0] != "PROJ-7") {
			t.Errorf("tickets of %q = %v, want [PROJ-7]", entry.Description, entry.Activity.Tickets)
		}
		if entry.Activity.Device != "laptop" {
			t.Errorf("device of %q = %q", entry.Description, entry.Activity.Device)
		}
	}
}

func TestCollectSkipsWithoutRoots(t *testing.T) {
	_, err := (&Collector{}).Collect(context.Background(), collector.Request{})
	if _, ok := err.(*collector.SkippedError); !ok {
		t.Fatalf("error = %v, want a skip", err)
	}
}

func TestDiscoverSkipsHiddenAndDependencyFolders(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"a/.git", "b/node_modules/c/.git", ".hidden/d/.git", "e/f/.git", "a/nested/.git"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0755); err != nil {
			t.Fatal(err)
		}
	}
	found, err := Discover([]string{root}, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(root, "a"), filepath.Join(root, "e", "f")}
	if len(found) != len(want) || found[0] != want[0] || found[1] != want[1] {
		t.Fatalf("Discover() = %v, want %v", found, want)
	}
}

func TestRepoNameFromURL(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:acme/widgets-api.git":         "acme/widgets-api",
		"https://github.com/Example/Widgets.Api.git":  "Example/Widgets.Api",
		"https://github.com/acme/widgets-docs":        "acme/widgets-docs",
		"ssh://git@github.com/acme/widgets.git":       "acme/widgets",
		"https://dev.azure.com/org/project/_git/repo": "project/repo",
		"/srv/git/widgets.git":                        "git/widgets",
		"https://example.com":                         "",
	} {
		if got := RepoNameFromURL(url); got != want {
			t.Errorf("RepoNameFromURL(%q) = %q, want %q", url, got, want)
		}
	}
}

func TestWalkReflogFollowsCheckoutsAndRebases(t *testing.T) {
	base := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	entries := []reflogEntry{
		{at: base, subject: "commit (initial): start", sha: "1"},
		{at: base.Add(time.Minute), subject: "checkout: moving from main to feature/x", sha: "1"},
		{at: base.Add(2 * time.Minute), subject: "commit: work", sha: "2"},
		{at: base.Add(3 * time.Minute), subject: "checkout: moving from feature/x to 0123456789abcdef0123456789abcdef01234567", sha: "3"},
		{at: base.Add(4 * time.Minute), subject: "rebase (pick): work", sha: "4"},
		{at: base.Add(5 * time.Minute), subject: "rebase (finish): returning to refs/heads/feature/x", sha: "4"},
	}
	touched := map[string]int{}
	commits := walkReflog(entries, func(branch string, _ time.Time) {
		if branch != "" {
			touched[branch]++
		}
	})
	if touched["feature/x"] != 3 || len(touched) != 1 {
		t.Errorf("touched = %v, want feature/x three times", touched)
	}
	if commits["2"] != "feature/x" || commits["1"] != "" {
		t.Errorf("commit branches = %v", commits)
	}
}
