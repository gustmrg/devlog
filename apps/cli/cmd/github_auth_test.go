package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveGitHubTokenPrefersEnvironment(t *testing.T) {
	t.Setenv("DEVLOG_TEST_GITHUB_TOKEN", "from-environment")
	token, source, err := resolveGitHubToken(context.Background(), "DEVLOG_TEST_GITHUB_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if token != "from-environment" || source != "environment" {
		t.Fatalf("got token %q from %q", token, source)
	}
}

func TestResolveGitHubTokenFallsBackToGitHubCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'from-gh\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("DEVLOG_TEST_GITHUB_TOKEN", "")
	token, source, err := resolveGitHubToken(context.Background(), "DEVLOG_TEST_GITHUB_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if token != "from-gh" || source != "gh" {
		t.Fatalf("got token %q from %q", token, source)
	}
}
