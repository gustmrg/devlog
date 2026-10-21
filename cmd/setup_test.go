package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"devlog/internal/collector"
	"devlog/internal/scheduler"

	"github.com/spf13/viper"
)

type recordedSetup struct {
	remoteURL string
	times     []scheduler.Clock
	args      []string
	envFile   string
	mcp       bool
}

// newSetupTest isolates HOME, Git, Claude Code, and viper, and loads the
// configuration the way the root command does.
func newSetupTest(t *testing.T, config string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	gitconfig := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(gitconfig, []byte("[user]\n\tname = Dev\n\temail = dev@example.com\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
	if err := os.MkdirAll(filepath.Join(home, "claude", "projects"), 0755); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err := os.MkdirAll(filepath.Join(home, ".devlog"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".devlog", "config.json"), []byte(config), 0644); err != nil {
			t.Fatal(err)
		}
	}
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.AddConfigPath(filepath.Join(home, ".devlog"))
	viper.SetConfigType("json")
	viper.SetConfigName("config")
	_ = viper.ReadInConfig()
	previousGOOS := setupGOOS
	setupGOOS = "linux"
	t.Cleanup(func() { setupGOOS = previousGOOS })
	return home
}

func stubActions(recorded *recordedSetup) setupActions {
	return setupActions{
		githubLogin: func(context.Context) (string, string, error) { return "octo", "gh", nil },
		initRemote: func(_ context.Context, url string) error {
			recorded.remoteURL = url
			viper.Set("remote.enabled", true)
			viper.Set("remote.url", url)
			return nil
		},
		schedule: func(times []scheduler.Clock, args []string, envFile string) (string, error) {
			recorded.times, recorded.args, recorded.envFile = times, args, envFile
			return "test scheduler", nil
		},
		installMCP:  func() error { recorded.mcp = true; return nil },
		testCollect: func(context.Context) []collector.Result { return []collector.Result{{Collector: "git"}} },
	}
}

func savedConfig(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".devlog", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func setting(config map[string]any, path string) any {
	var value any = config
	for _, key := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[key]
	}
	return value
}

func TestSetupConfiguresAWorkMachine(t *testing.T) {
	home := newSetupTest(t, "")
	repo := filepath.Join(home, "dev", "widgets")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %s", output)
	}
	input := strings.Join([]string{
		"",                    // role: work machine
		"laptop",              // device name
		"Mars/Base",           // invalid time zone, asked again
		"America/Los_Angeles", // time zone
		"",                    // ticket pattern: default
		"~/dev",               // repository folders
		"",                    // commit emails: suggested from Git
		"none",                // opencode
		"git@example.com:you/devlog-data.git",
		"",  // schedule: default times
		"n", // test collection
	}, "\n") + "\n"
	var recorded recordedSetup
	var out bytes.Buffer
	if err := runSetup(context.Background(), strings.NewReader(input), &out, stubActions(&recorded)); err != nil {
		t.Fatalf("setup failed: %v\n%s", err, out.String())
	}

	for _, expected := range []string{"unknown time zone", "found 1 repository", "Claude Code transcripts found", "Setup complete"} {
		if !strings.Contains(out.String(), expected) {
			t.Errorf("output does not contain %q:\n%s", expected, out.String())
		}
	}
	config := savedConfig(t, home)
	for path, want := range map[string]string{
		"device.name":              `"laptop"`,
		"collect.timezone":         `"America/Los_Angeles"`,
		"collect.ticketpattern":    jsonString(collector.DefaultTicketPattern),
		"collect.sources":          `["git","claude-code","opencode"]`,
		"collect.git.roots":        `["~/dev"]`,
		"collect.git.authoremails": `["dev@example.com"]`,
		"collect.opencode.command": `""`,
	} {
		if got := jsonString(setting(config, path)); got != want {
			t.Errorf("%s = %s, want %s", path, got, want)
		}
	}
	if recorded.remoteURL != "git@example.com:you/devlog-data.git" {
		t.Errorf("remote = %q", recorded.remoteURL)
	}
	if len(recorded.times) != 2 || recorded.times[0] != (scheduler.Clock{Hour: 12, Minute: 15}) || strings.Join(recorded.args, " ") != "collect --remote" {
		t.Errorf("schedule = %v %v", recorded.times, recorded.args)
	}
	if recorded.mcp {
		t.Error("a work machine was offered the MCP service")
	}
}

func TestSetupConfiguresAnAlwaysOnMachine(t *testing.T) {
	home := newSetupTest(t, "")
	input := strings.Join([]string{"2", "server", "none", "", "", "none", "", "y", "n"}, "\n") + "\n"
	var recorded recordedSetup
	var out bytes.Buffer
	if err := runSetup(context.Background(), strings.NewReader(input), &out, stubActions(&recorded)); err != nil {
		t.Fatalf("setup failed: %v\n%s", err, out.String())
	}
	config := savedConfig(t, home)
	if got := jsonString(setting(config, "collect.sources")); got != `["github"]` {
		t.Errorf("sources = %s", got)
	}
	if got := setting(config, "github.username"); got != "octo" {
		t.Errorf("github.username = %v, want the signed-in login", got)
	}
	if !strings.Contains(out.String(), "GitHub CLI is signed in as octo") || !recorded.mcp {
		t.Errorf("GitHub check or MCP install missing:\n%s", out.String())
	}
	if len(recorded.times) != 2 || recorded.times[0] != (scheduler.Clock{Hour: 12, Minute: 20}) || strings.Join(recorded.args, " ") != "collect" {
		t.Errorf("schedule = %v %v", recorded.times, recorded.args)
	}
}

func TestSetupKeepsSettingsItDoesNotAsk(t *testing.T) {
	home := newSetupTest(t, `{"llm":{"enabled":true,"model":"custom"},"remote":{"enabled":true,"url":"git@example.com:you/data.git"},"collect":{"sources":["github"]}}`)
	actions := stubActions(&recordedSetup{})
	actions.githubLogin = func(context.Context) (string, string, error) { return "", "", errors.New("no token") }
	input := strings.Join([]string{"", "server", "none", "", "octo", "none", "n", "n"}, "\n") + "\n"
	var out bytes.Buffer
	if err := runSetup(context.Background(), strings.NewReader(input), &out, actions); err != nil {
		t.Fatalf("setup failed: %v\n%s", err, out.String())
	}
	config := savedConfig(t, home)
	if setting(config, "llm.enabled") != true || setting(config, "llm.model") != "custom" || setting(config, "remote.url") != "git@example.com:you/data.git" {
		t.Errorf("settings setup does not ask about were lost: %v", config)
	}
	if !strings.Contains(out.String(), "syncing with git@example.com:you/data.git") || !strings.Contains(out.String(), "needs a token") {
		t.Errorf("unexpected output:\n%s", out.String())
	}
}

func TestSetupInterruptedBeforeSavingChangesNothing(t *testing.T) {
	home := newSetupTest(t, "")
	var out bytes.Buffer
	err := runSetup(context.Background(), strings.NewReader("1\nlaptop\n"), &out, stubActions(&recordedSetup{}))
	if err == nil || !strings.Contains(err.Error(), "no changes were saved") {
		t.Fatalf("error = %v, want an interruption before saving", err)
	}
	if setting(savedConfig(t, home), "device.name") != nil {
		t.Error("an interrupted setup saved its answers")
	}
}

func TestSaveEnvValueReplacesTheKeyAndKeepsOtherLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collect.env")
	if err := os.WriteFile(path, []byte("# tokens\nGITHUB_TOKEN=old\nOTHER=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveEnvValue(path, "GITHUB_TOKEN", "new"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# tokens\nOTHER=1\nGITHUB_TOKEN=new\n" {
		t.Fatalf("env file = %q", data)
	}
	if info, err := os.Stat(path); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func jsonString(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
