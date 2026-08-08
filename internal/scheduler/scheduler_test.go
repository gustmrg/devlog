package scheduler

import (
	"strings"
	"testing"
)

func TestParseTime(t *testing.T) {
	hour, minute, err := ParseTime("07:05")
	if err != nil || hour != 7 || minute != 5 {
		t.Fatalf("ParseTime() = %d, %d, %v", hour, minute, err)
	}
	for _, value := range []string{"7", "24:00", "12:60", "aa:10"} {
		if _, _, err := ParseTime(value); err == nil {
			t.Errorf("ParseTime(%q) unexpectedly succeeded", value)
		}
	}
}

func TestLaunchdDefinition(t *testing.T) {
	opts := Options{
		Executable: "/Applications/Dev Log/devlog",
		Hour:       23,
		Minute:     55,
		Polish:     true,
		Remote:     true,
		EnvFile:    "/Users/test/.devlog/sync.env",
		LogFile:    "/Users/test/.devlog/sync.log",
	}
	definition := launchdDefinition(opts)
	for _, expected := range []string{
		"<string>/Applications/Dev Log/devlog</string>",
		"<string>--polish</string>",
		"<string>--remote</string>",
		"<string>--env-file</string>",
		"<key>Hour</key><integer>23</integer>",
		"<key>Minute</key><integer>55</integer>",
	} {
		if !strings.Contains(definition, expected) {
			t.Errorf("launchd definition does not contain %q", expected)
		}
	}
}

func TestCronDefinitionQuotesPaths(t *testing.T) {
	opts := Options{
		Executable: "/home/test/dev log/devlog",
		Hour:       8,
		Minute:     30,
		Remote:     true,
		EnvFile:    "/home/test/it's.env",
		LogFile:    "/home/test/dev log/sync.log",
	}
	definition := cronDefinition(opts)
	if !strings.HasPrefix(definition, "30 8 * * * ") {
		t.Fatalf("unexpected cron schedule: %s", definition)
	}
	if !strings.Contains(definition, `'/home/test/dev log/devlog'`) {
		t.Errorf("executable path was not quoted: %s", definition)
	}
	if !strings.Contains(definition, `'/home/test/it'"'"'s.env'`) {
		t.Errorf("single quote was not escaped: %s", definition)
	}
	if !strings.Contains(definition, " sync --remote --env-file ") {
		t.Errorf("remote sync arguments are missing: %s", definition)
	}
}

func TestManagedCronBlockReplacement(t *testing.T) {
	content := "MAILTO=user@example.com\n\n" + cronBegin + "\nold job\n" + cronEnd + "\n\n0 1 * * * backup\n"
	block := managedCronBlock(content)
	if !strings.Contains(block, "old job") {
		t.Fatalf("managed block not found: %q", block)
	}
	remaining := removeManagedCronBlock(content)
	if strings.Contains(remaining, "old job") || !strings.Contains(remaining, "MAILTO=user@example.com") || !strings.Contains(remaining, "backup") {
		t.Fatalf("unexpected remaining crontab: %q", remaining)
	}
}
