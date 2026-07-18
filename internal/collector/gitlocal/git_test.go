package gitlocal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDiscoverAndCollectMetadata(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "init")
	run(t, repo, "config", "user.email", "devlog@example.invalid")
	run(t, repo, "config", "user.name", "DevLog Test")
	if err := os.WriteFile(filepath.Join(repo, "work.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", "work.txt")
	run(t, repo, "commit", "-m", "initial")
	repos, err := Discover([]string{root}, 3)
	if err != nil || len(repos) != 1 {
		t.Fatalf("repos=%v err=%v", repos, err)
	}
	collector := Collector{Root: repo, DeviceID: "device", ProjectID: "project"}
	events, cursor, err := collector.Collect(context.Background(), "")
	if err != nil || len(events) != 1 || cursor == "" {
		t.Fatalf("events=%d cursor=%q err=%v", len(events), cursor, err)
	}
	again, _, err := collector.Collect(context.Background(), cursor)
	if err != nil || len(again) != 0 {
		t.Fatalf("again=%d err=%v", len(again), err)
	}
	if string(events[0].Payload) == "content" {
		t.Fatal("file content leaked")
	}
}
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
