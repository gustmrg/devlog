package scheduler

import (
	"strings"
	"testing"
	"unicode/utf16"
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

func TestParseTimes(t *testing.T) {
	times, err := ParseTimes("12:15, 18:15")
	if err != nil || len(times) != 2 || times[0] != (Clock{12, 15}) || times[1] != (Clock{18, 15}) {
		t.Fatalf("ParseTimes() = %v, %v", times, err)
	}
	for _, value := range []string{"", ",", "12:15,25:00"} {
		if _, err := ParseTimes(value); err == nil {
			t.Errorf("ParseTimes(%q) unexpectedly succeeded", value)
		}
	}
}

func TestLaunchdDefinition(t *testing.T) {
	opts := Options{
		Name:       "sync",
		Executable: "/Applications/Dev Log/devlog",
		Times:      []Clock{{23, 55}},
		Args:       []string{"sync", "--polish", "--remote", "--env-file", "/Users/test/.devlog/sync.env"},
		LogFile:    "/Users/test/.devlog/sync.log",
	}
	definition := launchdDefinition(opts)
	for _, expected := range []string{
		"<string>com.gustmrg.devlog.sync</string>",
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
	if strings.Contains(definition, "Weekday") {
		t.Error("a daily job should not be limited to weekdays")
	}
}

func TestLaunchdDefinitionRunsOnWeekdays(t *testing.T) {
	definition := launchdDefinition(Options{Name: "collect", Executable: "/usr/local/bin/devlog", Times: []Clock{{12, 15}, {18, 15}}, Weekdays: true, Args: []string{"collect"}, LogFile: "/tmp/collect.log"})
	if got := strings.Count(definition, "<key>Weekday</key>"); got != 10 {
		t.Errorf("weekday intervals = %d, want 5 days for each of 2 times", got)
	}
	if strings.Contains(definition, "<key>Weekday</key><integer>0</integer>") || strings.Contains(definition, "<key>Weekday</key><integer>6</integer>") {
		t.Error("the job should not run on weekends")
	}
}

func TestCronDefinitionQuotesPaths(t *testing.T) {
	opts := Options{
		Name:       "sync",
		Executable: "/home/test/dev log/devlog",
		Times:      []Clock{{8, 30}},
		Args:       []string{"sync", "--remote", "--env-file", "/home/test/it's.env"},
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

func TestCronDefinitionRunsEachTimeOnWeekdays(t *testing.T) {
	definition := cronDefinition(Options{Name: "collect", Executable: "/usr/local/bin/devlog", Times: []Clock{{12, 20}, {18, 20}}, Weekdays: true, Args: []string{"collect", "--sources", "github"}, LogFile: "/home/dev/.devlog/collect.log"})
	lines := strings.Split(definition, "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "20 12 * * 1-5 /usr/local/bin/devlog collect --sources github >> ") || !strings.HasPrefix(lines[1], "20 18 * * 1-5 ") {
		t.Fatalf("unexpected cron definition:\n%s", definition)
	}
}

func TestManagedCronBlockReplacement(t *testing.T) {
	content := "MAILTO=user@example.com\n\n" + cronBegin("sync") + "\nold job\n" + cronEnd("sync") + "\n\n" +
		cronBegin("collect") + "\ncollect job\n" + cronEnd("collect") + "\n\n0 1 * * * backup\n"
	block := managedCronBlock(content, "sync")
	if !strings.Contains(block, "old job") || strings.Contains(block, "collect job") {
		t.Fatalf("managed block not found: %q", block)
	}
	remaining := removeManagedCronBlock(content, "sync")
	if strings.Contains(remaining, "old job") || !strings.Contains(remaining, "collect job") || !strings.Contains(remaining, "MAILTO=user@example.com") || !strings.Contains(remaining, "backup") {
		t.Fatalf("unexpected remaining crontab: %q", remaining)
	}
}

func TestTaskDefinition(t *testing.T) {
	definition := taskDefinition(Options{
		Name:       "collect",
		Executable: `C:\Users\Dev Box\AppData\Local\Programs\devlog\devlog.exe`,
		Times:      []Clock{{12, 15}, {18, 15}},
		Weekdays:   true,
		Args:       []string{"collect", "--remote"},
		LogFile:    `C:\Users\Dev Box\.devlog\collect.log`,
	})
	for _, expected := range []string{
		"<StartBoundary>2026-01-01T12:15:00</StartBoundary>",
		"<StartBoundary>2026-01-01T18:15:00</StartBoundary>",
		"<Monday /><Tuesday /><Wednesday /><Thursday /><Friday />",
		"<StartWhenAvailable>true</StartWhenAvailable>",
		`<Command>C:\Users\Dev Box\AppData\Local\Programs\devlog\devlog.exe</Command>`,
		`<Arguments>collect --remote --log-file &#34;C:\Users\Dev Box\.devlog\collect.log&#34;</Arguments>`,
	} {
		if !strings.Contains(definition, expected) {
			t.Errorf("task definition does not contain %q:\n%s", expected, definition)
		}
	}
	if strings.Contains(definition, "Saturday") || strings.Contains(definition, "Sunday") {
		t.Error("the task should not run on weekends")
	}
}

func TestUTF16LEStartsWithByteOrderMark(t *testing.T) {
	data := utf16LE("<é>")
	if data[0] != 0xFF || data[1] != 0xFE || len(data) != 8 {
		t.Fatalf("utf16LE() = %v", data)
	}
	units := make([]uint16, 0, 3)
	for i := 2; i < len(data); i += 2 {
		units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
	}
	if string(utf16.Decode(units)) != "<é>" {
		t.Fatalf("decoded %q", string(utf16.Decode(units)))
	}
}

func TestWindowsQuote(t *testing.T) {
	for arg, want := range map[string]string{
		"collect":            "collect",
		"":                   `""`,
		`C:\Dev Box\log.txt`: `"C:\Dev Box\log.txt"`,
		`C:\Dev Box\`:        `"C:\Dev Box\\"`,
		`say "hi"`:           `"say \"hi\""`,
	} {
		if got := windowsQuote(arg); got != want {
			t.Errorf("windowsQuote(%q) = %s, want %s", arg, got, want)
		}
	}
}
