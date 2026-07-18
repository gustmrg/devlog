package gitlocal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"devlog/internal/domain"
	"github.com/google/uuid"
)

type Collector struct{ Root, DeviceID, ProjectID string }

func (c Collector) Type() string { return "git" }
func (c Collector) Validate() error {
	if c.Root == "" {
		return fmt.Errorf("git root is required")
	}
	return nil
}

func (c Collector) Collect(ctx context.Context, cursor string) ([]domain.Event, string, error) {
	if err := c.Validate(); err != nil {
		return nil, cursor, err
	}
	root, err := filepath.Abs(c.Root)
	if err != nil {
		return nil, cursor, err
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return nil, cursor, fmt.Errorf("%s is not a git repository", root)
	}
	head, err := git(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return nil, cursor, err
	}
	branch, _ := git(ctx, root, "branch", "--show-current")
	remote, _ := git(ctx, root, "remote", "get-url", "origin")
	status, err := git(ctx, root, "status", "--porcelain=v1")
	if err != nil {
		return nil, cursor, err
	}
	var files []string
	for _, line := range strings.Split(status, "\n") {
		if len(line) > 3 {
			files = append(files, strings.TrimSpace(line[3:]))
		}
	}
	sort.Strings(files)
	payload := map[string]any{"head": head, "branch": branch, "remote": NormalizeRemote(remote), "files": files, "dirty": len(files) > 0}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	fingerprint := hex.EncodeToString(sum[:])
	nextCursor := head + "|" + fingerprint
	if nextCursor == cursor {
		return nil, cursor, nil
	}
	now := time.Now().UTC()
	previousHead := strings.SplitN(cursor, "|", 2)[0]
	kind := "git.snapshot"
	external := fingerprint
	if head != "" && head != previousHead {
		kind = "git.commit"
		external = head
		message, _ := git(ctx, root, "show", "-s", "--format=%s", head)
		commitAt, _ := git(ctx, root, "show", "-s", "--format=%cI", head)
		payload["message"] = message
		payload["commitAt"] = commitAt
		b, _ = json.Marshal(payload)
	}
	e := domain.Event{ID: uuid.NewString(), SourceType: "git", SourceInstanceID: c.ProjectID, ExternalID: external, DeviceID: c.DeviceID, ProjectID: c.ProjectID, Kind: kind, OccurredAt: now, ObservedAt: now, Payload: b, Fingerprint: "git:" + fingerprint}
	return []domain.Event{e}, nextCursor, nil
}

func Discover(roots []string, maxDepth int) ([]string, error) {
	seen := map[string]bool{}
	var repos []string
	for _, root := range roots {
		root = expand(root)
		baseDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}
			if !d.IsDir() {
				return nil
			}
			depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - baseDepth
			if depth > maxDepth {
				return filepath.SkipDir
			}
			if d.Name() == ".git" {
				repo := filepath.Dir(path)
				if !seen[repo] {
					seen[repo] = true
					repos = append(repos, repo)
				}
				return filepath.SkipDir
			}
			if d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == ".cache" {
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Strings(repos)
	return repos, nil
}

func NormalizeRemote(remote string) string {
	r := strings.TrimSpace(strings.TrimSuffix(remote, ".git"))
	r = strings.TrimPrefix(r, "git@")
	if i := strings.Index(r, ":"); i >= 0 && !strings.Contains(r[:i], "/") {
		r = r[:i] + "/" + r[i+1:]
	}
	r = strings.TrimPrefix(r, "https://")
	r = strings.TrimPrefix(r, "http://")
	r = strings.TrimPrefix(r, "ssh://")
	return r
}
func git(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}
func expand(path string) string {
	if strings.HasPrefix(path, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, path[2:])
		}
	}
	return path
}
