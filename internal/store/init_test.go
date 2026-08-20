package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestInitializeIsIdempotent(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	first, err := Initialize()
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created {
		t.Fatal("first initialization did not report a newly created config")
	}
	if first.ConfigFile != filepath.Join(home, ".devlog", "config.json") {
		t.Fatalf("unexpected config path: %s", first.ConfigFile)
	}

	original, err := os.ReadFile(first.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Initialize()
	if err != nil {
		t.Fatal(err)
	}
	if second.Created {
		t.Fatal("second initialization reported a newly created config")
	}
	unchanged, err := os.ReadFile(second.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(original) {
		t.Fatal("existing config was changed")
	}
}
