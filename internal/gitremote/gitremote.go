// Package gitremote synchronizes a DevLog data repository through Git.
package gitremote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"devlog/internal/store"
)

const maxPushAttempts = 3

type Manager struct {
	Repository *store.Repository
	URL        string
	Branch     string
}

type Status struct {
	Initialized bool
	URL         string
	Branch      string
	Dirty       bool
	Ahead       int
	Behind      int
}

func New(repository *store.Repository, url, branch string) *Manager {
	if branch == "" {
		branch = "main"
	}
	return &Manager{Repository: repository, URL: url, Branch: branch}
}

func (m *Manager) Init(ctx context.Context) error {
	if m.URL == "" {
		return fmt.Errorf("remote repository URL is required; run devlog remote init <url>")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("Git is required for remote synchronization but was not found in PATH")
	}
	unlock, err := m.Repository.Lock()
	if err != nil {
		return err
	}
	defer unlock()

	if !m.isGitRepository() {
		if _, err := m.git(ctx, "init", "-b", m.Branch); err != nil {
			if _, fallbackErr := m.git(ctx, "init"); fallbackErr != nil {
				return err
			}
			if _, err := m.git(ctx, "checkout", "-b", m.Branch); err != nil {
				return err
			}
		}
	}
	if _, err := m.git(ctx, "remote", "get-url", "origin"); err == nil {
		if _, err := m.git(ctx, "remote", "set-url", "origin", m.URL); err != nil {
			return err
		}
	} else if _, err := m.git(ctx, "remote", "add", "origin", m.URL); err != nil {
		return err
	}
	if err := m.commit(ctx, "Initialize DevLog data repository"); err != nil {
		return err
	}
	return m.syncLocked(ctx)
}

func (m *Manager) Sync(ctx context.Context) error {
	if !m.isGitRepository() || !m.hasOrigin(ctx) {
		return fmt.Errorf("remote sync is not initialized; run devlog remote init <url>")
	}
	unlock, err := m.Repository.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	return m.syncLocked(ctx)
}

func (m *Manager) syncLocked(ctx context.Context) error {
	if err := m.commit(ctx, "Sync local DevLog changes"); err != nil {
		return err
	}
	for attempt := 1; attempt <= maxPushAttempts; attempt++ {
		exists, err := m.remoteBranchExists(ctx)
		if err != nil {
			return err
		}
		if exists {
			if _, err := m.git(ctx, "fetch", "origin", m.Branch); err != nil {
				return err
			}
			if err := m.mergeRemote(ctx); err != nil {
				return err
			}
		}
		if _, err := m.git(ctx, "push", "-u", "origin", "HEAD:"+m.Branch); err == nil {
			return nil
		} else if attempt == maxPushAttempts {
			return fmt.Errorf("could not push DevLog data after %d attempts: %w", maxPushAttempts, err)
		}
	}
	return nil
}

func (m *Manager) mergeRemote(ctx context.Context) error {
	_, err := m.git(ctx, "-c", "user.name=DevLog", "-c", "user.email=devlog@local", "merge", "--no-edit", "--allow-unrelated-histories", "origin/"+m.Branch)
	if err == nil {
		if err := m.reconcileDuplicateSources(); err != nil {
			return err
		}
		return m.commit(ctx, "Reconcile remote DevLog changes")
	}
	conflicts, conflictErr := m.git(ctx, "diff", "--name-only", "--diff-filter=U", "-z")
	if conflictErr != nil || len(conflicts) == 0 {
		return err
	}
	for _, path := range bytes.Split(bytes.TrimRight(conflicts, "\x00"), []byte{0}) {
		if len(path) == 0 {
			continue
		}
		if err := m.resolveConflict(ctx, string(path)); err != nil {
			_, _ = m.git(ctx, "merge", "--abort")
			return err
		}
	}
	if err := m.reconcileDuplicateSources(); err != nil {
		return err
	}
	if _, err := m.git(ctx, "add", "-A"); err != nil {
		return err
	}
	return m.commit(ctx, "Merge remote DevLog changes")
}

func (m *Manager) resolveConflict(ctx context.Context, path string) error {
	ours, oursErr := m.stageFile(ctx, 2, path)
	theirs, theirsErr := m.stageFile(ctx, 3, path)
	if oursErr != nil && theirsErr != nil {
		return fmt.Errorf("could not read either side of Git conflict %q", path)
	}

	var winner []byte
	switch {
	case strings.HasPrefix(path, "entries/") && strings.HasSuffix(path, ".json"):
		winner = newerEntry(ours, theirs)
	case strings.HasPrefix(path, "summaries/") && strings.HasSuffix(path, ".md"):
		var loser []byte
		winner, loser = newerSummary(ours, theirs)
		if len(loser) > 0 && !bytes.Equal(winner, loser) {
			if err := m.preserveSummaryConflict(path, loser); err != nil {
				return err
			}
		}
	case path == ".devlog-version" || path == ".gitignore":
		winner = ours
		if len(winner) == 0 {
			winner = theirs
		}
	default:
		return fmt.Errorf("cannot automatically resolve Git conflict in unexpected file %q", path)
	}
	if len(winner) == 0 {
		if err := os.Remove(filepath.Join(m.Repository.Root, path)); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := atomicWrite(filepath.Join(m.Repository.Root, path), winner); err != nil {
		return err
	}
	_, err := m.git(ctx, "add", "--", path)
	return err
}

func (m *Manager) stageFile(ctx context.Context, stage int, path string) ([]byte, error) {
	data, err := m.git(ctx, "show", fmt.Sprintf(":%d:%s", stage, path))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func newerEntry(a, b []byte) []byte {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	var first, second store.Entry
	if json.Unmarshal(a, &first) != nil {
		return b
	}
	if json.Unmarshal(b, &second) != nil {
		return a
	}
	firstTime := first.UpdatedAt
	if firstTime.IsZero() {
		firstTime = first.CreatedAt
	}
	secondTime := second.UpdatedAt
	if secondTime.IsZero() {
		secondTime = second.CreatedAt
	}
	if secondTime.After(firstTime) || secondTime.Equal(firstTime) && bytes.Compare(b, a) > 0 {
		return b
	}
	return a
}

func newerSummary(a, b []byte) ([]byte, []byte) {
	if len(a) == 0 {
		return b, nil
	}
	if len(b) == 0 {
		return a, nil
	}
	first, firstErr := store.ParseSummary(a)
	second, secondErr := store.ParseSummary(b)
	if firstErr != nil {
		return b, a
	}
	if secondErr != nil {
		return a, b
	}
	if second.GeneratedAt.After(first.GeneratedAt) || second.GeneratedAt.Equal(first.GeneratedAt) && bytes.Compare(b, a) > 0 {
		return b, a
	}
	return a, b
}

func (m *Manager) preserveSummaryConflict(path string, content []byte) error {
	summary, _ := store.ParseSummary(content)
	device := summary.DeviceID
	if device == "" {
		device = "unknown"
	}
	device = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, device)
	name := strings.TrimSuffix(filepath.Base(path), ".md")
	conflictPath := filepath.Join(m.Repository.Root, "summaries", "conflicts", fmt.Sprintf("%s.%s.%d.md", name, device, time.Now().UTC().UnixNano()))
	return atomicWrite(conflictPath, content)
}

func (m *Manager) reconcileDuplicateSources() error {
	root := filepath.Join(m.Repository.Root, "entries")
	type found struct {
		path string
		data []byte
	}
	byDateAndSource := map[string]found{}
	return filepath.WalkDir(root, func(path string, item os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if item.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var entry store.Entry
		if json.Unmarshal(data, &entry) != nil || entry.Source == "" {
			return nil
		}
		key := filepath.Dir(path) + "\x00" + entry.Source
		previous, exists := byDateAndSource[key]
		if !exists {
			byDateAndSource[key] = found{path: path, data: data}
			return nil
		}
		winner := newerEntry(previous.data, data)
		if bytes.Equal(winner, previous.data) {
			return os.Remove(path)
		}
		if err := os.Remove(previous.path); err != nil {
			return err
		}
		byDateAndSource[key] = found{path: path, data: data}
		return nil
	})
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	status := Status{URL: m.URL, Branch: m.Branch}
	if !m.isGitRepository() || !m.hasOrigin(ctx) {
		return status, nil
	}
	status.Initialized = true
	if output, err := m.git(ctx, "remote", "get-url", "origin"); err == nil {
		status.URL = strings.TrimSpace(string(output))
	}
	output, err := m.git(ctx, "status", "--porcelain")
	if err != nil {
		return status, err
	}
	status.Dirty = len(bytes.TrimSpace(output)) > 0
	if _, err := m.git(ctx, "rev-parse", "--verify", "origin/"+m.Branch); err == nil {
		counts, err := m.git(ctx, "rev-list", "--left-right", "--count", "origin/"+m.Branch+"...HEAD")
		if err == nil {
			_, _ = fmt.Sscanf(string(counts), "%d %d", &status.Behind, &status.Ahead)
		}
	}
	return status, nil
}

func (m *Manager) Disconnect(ctx context.Context) error {
	if !m.isGitRepository() {
		return nil
	}
	if _, err := m.git(ctx, "remote", "get-url", "origin"); err != nil {
		return nil
	}
	_, err := m.git(ctx, "remote", "remove", "origin")
	return err
}

func (m *Manager) commit(ctx context.Context, message string) error {
	if _, err := m.git(ctx, "add", "-A"); err != nil {
		return err
	}
	cmd := m.command(ctx, "diff", "--cached", "--quiet")
	if err := cmd.Run(); err == nil {
		if _, mergeErr := os.Stat(filepath.Join(m.Repository.Root, ".git", "MERGE_HEAD")); os.IsNotExist(mergeErr) {
			return nil
		}
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return err
		}
	}
	_, err := m.git(ctx, "-c", "user.name=DevLog", "-c", "user.email=devlog@local", "commit", "-m", message)
	return err
}

func (m *Manager) remoteBranchExists(ctx context.Context) (bool, error) {
	cmd := m.command(ctx, "ls-remote", "--exit-code", "--heads", "origin", m.Branch)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return len(bytes.TrimSpace(output)) > 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return false, nil
	}
	return false, commandError(cmd, output, err)
}

func (m *Manager) isGitRepository() bool {
	info, err := os.Stat(filepath.Join(m.Repository.Root, ".git"))
	return err == nil && info.IsDir()
}

func (m *Manager) hasOrigin(ctx context.Context) bool {
	_, err := m.git(ctx, "remote", "get-url", "origin")
	return err == nil
}

func (m *Manager) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.Repository.Root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd
}

func (m *Manager) git(ctx context.Context, args ...string) ([]byte, error) {
	cmd := m.command(ctx, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, commandError(cmd, output, err)
	}
	return output, nil
}

func commandError(cmd *exec.Cmd, output []byte, err error) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	rawArgs := cmd.Args[1:]
	safeArgs := safeGitArgs(rawArgs)
	for i := range rawArgs {
		if safeArgs[i] != rawArgs[i] {
			message = strings.ReplaceAll(message, rawArgs[i], safeArgs[i])
		}
	}
	return fmt.Errorf("git %s failed: %s", strings.Join(safeArgs, " "), message)
}

func safeGitArgs(args []string) []string {
	safe := append([]string(nil), args...)
	if len(safe) >= 4 && safe[0] == "remote" && (safe[1] == "add" || safe[1] == "set-url") {
		safe[3] = "<url>"
	}
	return safe
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".devlog-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
