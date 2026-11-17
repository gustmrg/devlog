package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMCPUnitQuotesPaths(t *testing.T) {
	unit := mcpUnit("/home/dev/my bin/devlog", "127.0.0.1:8766", "/home/dev/.devlog/mcp-token")
	if !strings.Contains(unit, `ExecStart="/home/dev/my bin/devlog" mcp --http 127.0.0.1:8766 --token-file /home/dev/.devlog/mcp-token`) {
		t.Fatalf("unexpected unit:\n%s", unit)
	}
	if !strings.Contains(unit, "WantedBy=default.target") {
		t.Errorf("unit is not enabled for the user session:\n%s", unit)
	}
}

func TestValidateListenAddress(t *testing.T) {
	for address, valid := range map[string]bool{
		"127.0.0.1:8766": true, "localhost:1": true, ":8766": false, "127.0.0.1": false, "127.0.0.1:0": false, "127.0.0.1:70000": false,
	} {
		if err := validateListenAddress(address); (err == nil) != valid {
			t.Errorf("validateListenAddress(%q) = %v, want valid %t", address, err, valid)
		}
	}
}

func TestMCPTokenFileIsCreatedOnceAndPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-token")
	first, err := mcpTokenFile(path, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := mcpTokenFile(path, false)
	if err != nil || second != first || len(first) < 32 {
		t.Fatalf("tokens = %q and %q (%v), want the same long token", first, second, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("token file mode = %v, %v", info.Mode(), err)
	}
	if _, err := mcpTokenFile(filepath.Join(t.TempDir(), "missing"), true); err == nil {
		t.Error("a missing explicit token file was created")
	}
}

type systemctlStub struct {
	calls  []string
	active string
	fail   bool
}

func stubSystemctl(t *testing.T, stub *systemctlStub) {
	t.Helper()
	previous := runSystemCommand
	runSystemCommand = func(name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		stub.calls = append(stub.calls, call)
		switch {
		case strings.Contains(call, "is-active"):
			return stub.active + "\n", nil
		case strings.Contains(call, "restart") && stub.fail:
			return "Access denied", errors.New("exit status 1")
		}
		return "", nil
	}
	previousGOOS, previousEUID := serviceGOOS, currentEUID
	serviceGOOS, currentEUID = "linux", func() int { return 1000 }
	t.Cleanup(func() { runSystemCommand, serviceGOOS, currentEUID = previous, previousGOOS, previousEUID })
}

func installTestUnit(t *testing.T, configHome string) {
	t.Helper()
	dir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, mcpServiceName), []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRestartsTheRunningMCPService(t *testing.T) {
	for _, test := range []struct {
		name      string
		installed bool
		active    string
		fail      bool
		message   string
		ok        bool
		restarted bool
	}{
		{name: "not installed", installed: false, ok: true},
		{name: "running", installed: true, active: "active", message: "restarted the MCP service", ok: true, restarted: true},
		{name: "stopped", installed: true, active: "inactive", message: "stopped", ok: true},
		{name: "restart fails", installed: true, active: "active", fail: true, message: "run `systemctl --user restart devlog-mcp`", ok: false, restarted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			configHome := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("SUDO_USER", "")
			if test.installed {
				installTestUnit(t, configHome)
			}
			stub := &systemctlStub{active: test.active, fail: test.fail}
			stubSystemctl(t, stub)

			message, ok := restartMCPServiceAfterUpdate()
			if ok != test.ok || !strings.Contains(message, test.message) || (test.message == "") != (message == "") {
				t.Errorf("restartMCPServiceAfterUpdate() = %q, %t", message, ok)
			}
			restarted := slices.Contains(stub.calls, "systemctl --user restart "+mcpServiceName)
			if restarted != test.restarted {
				t.Errorf("calls = %v, restarted = %t, want %t", stub.calls, restarted, test.restarted)
			}
			if !test.installed && len(stub.calls) > 0 {
				t.Errorf("systemctl was called without an installed service: %v", stub.calls)
			}
		})
	}
}

func TestUpdateUnderSudoRestartsTheInvokingUsersService(t *testing.T) {
	home := t.TempDir()
	installTestUnit(t, filepath.Join(home, ".config"))
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("SUDO_USER", "dev")
	stub := &systemctlStub{active: "active"}
	stubSystemctl(t, stub)
	currentEUID = func() int { return 0 }
	previousLookup := lookupHome
	lookupHome = func(name string) (string, error) {
		if name != "dev" {
			return "", errors.New("unknown user")
		}
		return home, nil
	}
	t.Cleanup(func() { lookupHome = previousLookup })

	message, ok := restartMCPServiceAfterUpdate()
	if !ok || !strings.Contains(message, "restarted") {
		t.Fatalf("restartMCPServiceAfterUpdate() = %q, %t", message, ok)
	}
	want := []string{
		"systemctl --user --machine=dev@ is-active " + mcpServiceName,
		"systemctl --user --machine=dev@ restart " + mcpServiceName,
	}
	if !slices.Equal(stub.calls, want) {
		t.Errorf("calls = %v, want %v", stub.calls, want)
	}
}
