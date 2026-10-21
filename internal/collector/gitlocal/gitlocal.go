// Package gitlocal collects activity from the Git repositories on this
// device: authored commits on every branch, branch activity recorded in the
// reflog, and the uncommitted and unpushed work of the checked-out branch.
package gitlocal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"devlog/internal/collector"
	"devlog/internal/store"
)

const (
	defaultMaxDepth = 4
	commandTimeout  = 30 * time.Second
	maxBodyLength   = 1500
	maxStatFiles    = 500
)

// skippedDirectories are never searched for repositories.
var skippedDirectories = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true, "bin": true, "obj": true,
}

// Collector reads the repositories found under Roots.
type Collector struct {
	Roots []string
	// AuthorEmails select the commits that are yours. When empty, each
	// repository's user.email is used.
	AuthorEmails []string
	MaxDepth     int
	Git          string
	now          func() time.Time
}

func (c *Collector) Name() string { return "git" }

func (c *Collector) Collect(ctx context.Context, request collector.Request) ([]store.Entry, error) {
	if len(c.Roots) == 0 {
		return nil, collector.Skip("no repository roots are configured; run devlog config set collect.git.roots <dir>[,<dir>...]")
	}
	git := c.git()
	if _, err := exec.LookPath(git); err != nil {
		return nil, collector.Skip("git is not installed")
	}
	repositories, err := Discover(c.Roots, c.MaxDepth)
	if err != nil {
		return nil, err
	}
	var entries []store.Entry
	var errs []error
	for _, dir := range repositories {
		repoEntries, err := c.collectRepository(ctx, dir, request)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", dir, err))
		}
		entries = append(entries, repoEntries...)
	}
	return entries, errors.Join(errs...)
}

func (c *Collector) git() string {
	if c.Git != "" {
		return c.Git
	}
	return "git"
}

func (c *Collector) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// Discover returns the Git repositories under roots, searching at most
// maxDepth directories deep and not inside repositories, hidden directories,
// or dependency and build folders.
func Discover(roots []string, maxDepth int) ([]string, error) {
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}
	var found []string
	for _, root := range roots {
		root, err := expandHome(root)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("could not read repository root %s: %w", root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("repository root %s is not a directory", root)
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if path == root {
					return err
				}
				return fs.SkipDir
			}
			if !entry.IsDir() {
				return nil
			}
			if path != root && (strings.HasPrefix(entry.Name(), ".") || skippedDirectories[entry.Name()]) {
				return fs.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				if !slices.Contains(found, path) {
					found = append(found, path)
				}
				return fs.SkipDir
			}
			if rel, err := filepath.Rel(root, path); err == nil && rel != "." && len(strings.Split(rel, string(filepath.Separator))) >= maxDepth {
				return fs.SkipDir
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("could not search repository root %s: %w", root, err)
		}
	}
	return found, nil
}

// Repository identifies a working tree.
type Repository struct {
	Dir string
	// Name is "owner/repo" from the remote URL, or the directory name.
	Name    string
	remotes []string
}

// Identify finds the repository that contains dir.
func Identify(ctx context.Context, git, dir string) (Repository, error) {
	if git == "" {
		git = "git"
	}
	top, err := run(ctx, git, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, err
	}
	repo := Repository{Dir: filepath.Clean(strings.TrimSpace(string(top)))}
	repo.Name = filepath.Base(repo.Dir)
	if output, err := run(ctx, git, repo.Dir, "remote"); err == nil {
		repo.remotes = strings.Fields(string(output))
	}
	remote := "origin"
	if !slices.Contains(repo.remotes, remote) && len(repo.remotes) > 0 {
		remote = repo.remotes[0]
	}
	if len(repo.remotes) > 0 {
		if output, err := run(ctx, git, repo.Dir, "remote", "get-url", remote); err == nil {
			if name := RepoNameFromURL(strings.TrimSpace(string(output))); name != "" {
				repo.Name = name
			}
		}
	}
	return repo, nil
}

var scpLikeURL = regexp.MustCompile(`^[^@/]+@[^:/]+:(.+)$`)

// RepoNameFromURL returns "owner/repo" for a remote URL, or "" when it has
// no such path.
func RepoNameFromURL(url string) string {
	path := url
	if match := scpLikeURL.FindStringSubmatch(url); match != nil {
		path = match[1]
	} else if _, rest, ok := strings.Cut(url, "://"); ok {
		if _, after, ok := strings.Cut(rest, "/"); ok {
			path = after
		} else {
			return ""
		}
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[len(parts)-2] == "_git" {
		// Azure DevOps: https://dev.azure.com/<org>/<project>/_git/<repo>
		parts = append(parts[:len(parts)-2], parts[len(parts)-1])
	}
	if len(parts) < 2 || parts[len(parts)-2] == "" || parts[len(parts)-1] == "" {
		return ""
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

type branchDay struct {
	day    string
	branch string
}

type branchActivity struct {
	first, last time.Time
	count       int
	changes     *store.Changes
}

func (a *branchActivity) add(t time.Time) {
	if a.count == 0 || t.Before(a.first) {
		a.first = t
	}
	if a.count == 0 || t.After(a.last) {
		a.last = t
	}
	a.count++
}

func (c *Collector) collectRepository(ctx context.Context, dir string, request collector.Request) ([]store.Entry, error) {
	git := c.git()
	window := request.Window
	repo, err := Identify(ctx, git, dir)
	if err != nil {
		return nil, err
	}
	emails := c.AuthorEmails
	if len(emails) == 0 {
		if output, err := run(ctx, git, repo.Dir, "config", "user.email"); err == nil && strings.TrimSpace(string(output)) != "" {
			emails = []string{strings.TrimSpace(string(output))}
		}
	}

	reflog, err := readReflog(ctx, git, repo.Dir)
	if err != nil {
		return nil, err
	}
	activity := map[branchDay]*branchActivity{}
	touch := func(branch string, t time.Time) {
		if branch == "" || !window.Contains(t) {
			return
		}
		key := branchDay{day: window.Day(t), branch: branch}
		if activity[key] == nil {
			activity[key] = &branchActivity{}
		}
		activity[key].add(t.In(window.Location))
	}
	commitBranches := walkReflog(reflog, touch)

	var entries []store.Entry
	if len(emails) > 0 {
		commits, err := c.commits(ctx, repo, emails, window, commitBranches)
		if err != nil {
			return nil, err
		}
		entries = append(entries, commits...)
	}

	now := c.clock()
	if window.Contains(now) {
		branch, changes, mtimes, err := workingTree(ctx, git, repo.Dir)
		if err != nil {
			return nil, err
		}
		if branch != "" {
			today := branchDay{day: window.Day(now), branch: branch}
			for _, mtime := range mtimes {
				if window.Day(mtime) == today.day {
					touch(branch, mtime)
				}
			}
			if current := activity[today]; current != nil && changes != nil {
				current.changes = changes
			}
		}
	}

	for key, branch := range activity {
		entries = append(entries, branchEntry(repo, key, branch, request.DeviceID))
	}
	return entries, nil
}

func branchEntry(repo Repository, key branchDay, activity *branchActivity, deviceID string) store.Entry {
	description := "Activity on branch " + key.branch
	if summary := describeChanges(activity.changes); summary != "" {
		description += "; " + summary
	}
	entry := store.Entry{
		Project:     collector.ShortRepoName(repo.Name),
		Description: description,
		Tags:        []string{"git", "branch"},
		Source:      fmt.Sprintf("git:branch:%s:%s:%s:%s", deviceID, repo.Name, key.branch, key.day),
		Activity: &store.Activity{
			Kind:       store.KindBranchActivity,
			Collector:  "git",
			OccurredAt: activity.first,
			Repo:       repo.Name,
			Branch:     key.branch,
			Directory:  repo.Dir,
			Count:      activity.count,
			Changes:    activity.changes,
		},
	}
	if activity.last.After(activity.first) {
		last := activity.last
		entry.Activity.EndedAt = &last
	}
	return entry
}

func describeChanges(changes *store.Changes) string {
	if changes == nil {
		return ""
	}
	var parts []string
	if changes.Files > 0 {
		parts = append(parts, fmt.Sprintf("%d uncommitted files (+%d −%d)", changes.Files, changes.Insertions, changes.Deletions))
	}
	if changes.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", changes.Untracked))
	}
	if changes.Unpushed > 0 {
		parts = append(parts, fmt.Sprintf("%d unpushed commits", changes.Unpushed))
	}
	return strings.Join(parts, ", ")
}

type reflogEntry struct {
	at      time.Time
	subject string
	sha     string
}

// readReflog returns HEAD's reflog, oldest first. A repository without one
// returns nothing.
func readReflog(ctx context.Context, git, dir string) ([]reflogEntry, error) {
	output, err := run(ctx, git, dir, "log", "--walk-reflogs", "--date=unix", "--format=%gd%x1f%gs%x1f%H", "-n", "20000", "HEAD")
	if err != nil {
		if _, headErr := run(ctx, git, dir, "rev-parse", "--verify", "HEAD"); headErr != nil {
			return nil, nil
		}
		return nil, err
	}
	var entries []reflogEntry
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\x1f")
		if len(fields) != 3 {
			continue
		}
		selector := fields[0]
		open, close := strings.LastIndex(selector, "{"), strings.LastIndex(selector, "}")
		if open < 0 || close < open {
			continue
		}
		seconds, err := strconv.ParseInt(selector[open+1:close], 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, reflogEntry{at: time.Unix(seconds, 0), subject: fields[1], sha: fields[2]})
	}
	slices.Reverse(entries)
	return entries, nil
}

var (
	checkoutSubject = regexp.MustCompile(`^checkout: moving from .+ to (.+)$`)
	finishSubject   = regexp.MustCompile(`\(finish\): returning to refs/heads/(.+)$`)
	fullSHA         = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
)

// walkReflog follows the checked-out branch through the reflog, reporting
// each operation on a branch to touch, and returns the branch every commit
// was made on.
func walkReflog(entries []reflogEntry, touch func(branch string, at time.Time)) map[string]string {
	commitBranches := map[string]string{}
	current := ""
	for _, entry := range entries {
		if match := checkoutSubject.FindStringSubmatch(entry.subject); match != nil {
			current = match[1]
			if fullSHA.MatchString(current) {
				current = ""
			}
		} else if match := finishSubject.FindStringSubmatch(entry.subject); match != nil {
			current = match[1]
		} else if strings.HasPrefix(entry.subject, "commit") && current != "" {
			commitBranches[entry.sha] = current
		}
		touch(current, entry.at)
	}
	return commitBranches
}

func (c *Collector) commits(ctx context.Context, repo Repository, emails []string, window collector.Window, commitBranches map[string]string) ([]store.Entry, error) {
	// --since filters by committer date, which is never earlier than the
	// author date, so it cannot drop a commit authored in the window.
	args := []string{"log", "--branches", "--remotes", "--no-merges", "--source", "--fixed-strings",
		"--since=" + window.From.Format(time.RFC3339), "--format=%H%x1f%aI%x1f%S%x1f%s%x1f%b%x1e"}
	for _, email := range emails {
		args = append(args, "--author="+email)
	}
	output, err := run(ctx, c.git(), repo.Dir, args...)
	if err != nil {
		if _, headErr := run(ctx, c.git(), repo.Dir, "rev-parse", "--verify", "HEAD"); headErr != nil {
			return nil, nil
		}
		return nil, err
	}
	var entries []store.Entry
	for record := range strings.SplitSeq(string(output), "\x1e") {
		fields := strings.SplitN(strings.TrimLeft(record, "\r\n"), "\x1f", 5)
		if len(fields) != 5 {
			continue
		}
		sha, subject, body := fields[0], fields[3], strings.TrimSpace(fields[4])
		authored, err := time.Parse(time.RFC3339, fields[1])
		if err != nil || !window.Contains(authored) {
			continue
		}
		branch := commitBranches[sha]
		if branch == "" {
			branch = repo.branchFromRef(fields[2])
		}
		description := subject
		if body != "" {
			description += "\n\n" + collector.Truncate(body, maxBodyLength)
		}
		entries = append(entries, store.Entry{
			Project:     collector.ShortRepoName(repo.Name),
			Description: description,
			Tags:        []string{"git", "commit"},
			Source:      "commit:" + sha,
			Activity: &store.Activity{
				Kind:       store.KindCommit,
				Collector:  "git",
				OccurredAt: authored,
				Repo:       repo.Name,
				Branch:     branch,
				SHA:        sha,
				Directory:  repo.Dir,
			},
		})
	}
	return entries, nil
}

// branchFromRef turns the ref git log --source reports into a branch name.
func (r Repository) branchFromRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	for _, remote := range r.remotes {
		if branch, ok := strings.CutPrefix(ref, remote+"/"); ok {
			return branch
		}
	}
	if ref == "HEAD" {
		return ""
	}
	return ref
}

// workingTree returns the checked-out branch, its uncommitted and unpushed
// work, and when the changed files were last modified.
func workingTree(ctx context.Context, git, dir string) (string, *store.Changes, []time.Time, error) {
	output, err := run(ctx, git, dir, "status", "--porcelain=v2", "--branch", "-z")
	if err != nil {
		return "", nil, nil, err
	}
	var branch, oid string
	upstream := false
	changes := &store.Changes{}
	var paths []string
	records := strings.Split(string(output), "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		switch {
		case strings.HasPrefix(record, "# branch.head "):
			branch = strings.TrimPrefix(record, "# branch.head ")
		case strings.HasPrefix(record, "# branch.oid "):
			oid = strings.TrimPrefix(record, "# branch.oid ")
		case strings.HasPrefix(record, "# branch.upstream "):
			upstream = true
		case strings.HasPrefix(record, "# branch.ab "):
			var ahead, behind int
			if _, err := fmt.Sscanf(strings.TrimPrefix(record, "# branch.ab "), "+%d -%d", &ahead, &behind); err == nil {
				changes.Unpushed = ahead
			}
		case strings.HasPrefix(record, "1 "):
			changes.Files++
			paths = append(paths, field(record, 8))
		case strings.HasPrefix(record, "2 "):
			changes.Files++
			paths = append(paths, field(record, 9))
			i++ // the original path follows a rename or copy
		case strings.HasPrefix(record, "u "):
			changes.Files++
			paths = append(paths, field(record, 10))
		case strings.HasPrefix(record, "? "):
			changes.Untracked++
			paths = append(paths, strings.TrimPrefix(record, "? "))
		}
	}
	if branch == "(detached)" {
		return "", nil, nil, nil
	}
	if !upstream && oid != "" && oid != "(initial)" {
		if count, err := run(ctx, git, dir, "rev-list", "--count", "HEAD", "--not", "--remotes"); err == nil {
			changes.Unpushed, _ = strconv.Atoi(strings.TrimSpace(string(count)))
		}
	}
	if changes.Files > 0 && oid != "(initial)" {
		if stat, err := run(ctx, git, dir, "diff", "--shortstat", "HEAD"); err == nil {
			changes.Insertions, changes.Deletions = parseShortstat(string(stat))
		}
	}
	var mtimes []time.Time
	for _, path := range paths[:min(len(paths), maxStatFiles)] {
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path))); err == nil {
			mtimes = append(mtimes, info.ModTime())
		}
	}
	if *changes == (store.Changes{}) {
		changes = nil
	}
	return branch, changes, mtimes, nil
}

// field returns the porcelain v2 path that follows n space-separated fields.
func field(record string, n int) string {
	parts := strings.SplitN(record, " ", n+1)
	if len(parts) <= n {
		return ""
	}
	return parts[n]
}

var shortstatNumber = regexp.MustCompile(`(\d+) (insertion|deletion)`)

func parseShortstat(stat string) (insertions, deletions int) {
	for _, match := range shortstatNumber.FindAllStringSubmatch(stat, -1) {
		value, _ := strconv.Atoi(match[1])
		if match[2] == "insertion" {
			insertions = value
		} else {
			deletions = value
		}
	}
	return insertions, deletions
}

func run(ctx context.Context, git, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, git, append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return output, fmt.Errorf("git %s failed: %s", args[0], message)
	}
	return output, nil
}

func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	return filepath.Abs(path)
}
