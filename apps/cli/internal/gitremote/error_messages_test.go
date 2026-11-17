package gitremote

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandErrorRedactsRemoteURL(t *testing.T) {
	const secretURL = "https://token@example.com/private.git"
	cmd := exec.Command("git", "remote", "add", "origin", secretURL)
	err := commandError(cmd, []byte("fatal: unable to access "+secretURL), errors.New("exit status 1"))

	if strings.Contains(err.Error(), secretURL) || strings.Contains(err.Error(), "token") {
		t.Fatalf("error exposes remote credentials: %q", err)
	}
	if !strings.Contains(err.Error(), "remote add origin <url>") {
		t.Fatalf("error does not describe the failed command: %q", err)
	}
}
