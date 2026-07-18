package config

import (
	"path/filepath"
	"testing"
)

func TestConfigRoundTripUsesStableJSONFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Default()
	cfg.Projects = []ProjectConfig{{ID: "devlog", Name: "DevLog", Path: "~/Dev/devlog", Enabled: true}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Projects[0].ID != "devlog" || loaded.Schedules.SummaryAt != "18:00" {
		t.Fatalf("unexpected config: %+v", loaded)
	}
}
