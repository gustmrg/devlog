package cmd

import (
	"os"
	"path/filepath"
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
