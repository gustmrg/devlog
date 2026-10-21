// Package scheduler installs and manages DevLog's platform-native scheduled
// jobs: launchd on macOS, the user's crontab on Linux, and Task Scheduler on
// Windows.
package scheduler

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Clock is a time of day.
type Clock struct {
	Hour   int
	Minute int
}

func (c Clock) String() string { return fmt.Sprintf("%02d:%02d", c.Hour, c.Minute) }

// Options describe one scheduled job.
type Options struct {
	// Name identifies the job, such as "sync" or "collect". Installing a job
	// replaces the previous job with the same name.
	Name       string
	Executable string
	Times      []Clock
	// Weekdays runs the job Monday to Friday only.
	Weekdays bool
	// Args are the DevLog arguments the job runs.
	Args    []string
	LogFile string
}

func ParseTime(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q: expected HH:MM", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("invalid time %q: hour must be between 00 and 23", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid time %q: minute must be between 00 and 59", value)
	}
	return hour, minute, nil
}

// ParseTimes parses comma-separated HH:MM times.
func ParseTimes(value string) ([]Clock, error) {
	var times []Clock
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		hour, minute, err := ParseTime(part)
		if err != nil {
			return nil, err
		}
		times = append(times, Clock{Hour: hour, Minute: minute})
	}
	if len(times) == 0 {
		return nil, fmt.Errorf("at least one time is required, such as 18:00")
	}
	return times, nil
}

func Install(opts Options) (string, error) {
	if err := validate(opts); err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(opts)
	case "linux":
		return installCron(opts)
	case "windows":
		return installTask(opts)
	default:
		return "", unsupported()
	}
}

func Show(name string) (string, bool, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchdPath(name)
		if err != nil {
			return "", false, err
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return string(data), err == nil, err
	case "linux":
		current, err := readCrontab()
		if err != nil {
			return "", false, err
		}
		block := managedCronBlock(current, name)
		return block, block != "", nil
	case "windows":
		output, err := exec.Command("schtasks", "/Query", "/TN", taskName(name), "/XML").CombinedOutput()
		if err != nil {
			return "", false, nil
		}
		return string(output), true, nil
	default:
		return "", false, unsupported()
	}
}

func Remove(name string) (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchdPath(name)
		if err != nil {
			return false, err
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false, nil
		}
		_ = exec.Command("launchctl", "unload", path).Run()
		return true, os.Remove(path)
	case "linux":
		current, err := readCrontab()
		if err != nil {
			return false, err
		}
		if managedCronBlock(current, name) == "" {
			return false, nil
		}
		return true, writeCrontab(removeManagedCronBlock(current, name))
	case "windows":
		if _, installed, _ := Show(name); !installed {
			return false, nil
		}
		if output, err := exec.Command("schtasks", "/Delete", "/TN", taskName(name), "/F").CombinedOutput(); err != nil {
			return false, fmt.Errorf("could not delete the scheduled task: %s", commandOutput(output, err))
		}
		return true, nil
	default:
		return false, unsupported()
	}
}

func unsupported() error {
	return fmt.Errorf("scheduled jobs are not supported on %s; supported platforms are macOS, Linux, and Windows", runtime.GOOS)
}

func validate(opts Options) error {
	if opts.Name == "" {
		return fmt.Errorf("scheduled job name is required")
	}
	if opts.Executable == "" || !filepath.IsAbs(opts.Executable) {
		return fmt.Errorf("DevLog executable path must be absolute")
	}
	if len(opts.Times) == 0 {
		return fmt.Errorf("at least one schedule time is required")
	}
	for _, clock := range opts.Times {
		if clock.Hour < 0 || clock.Hour > 23 || clock.Minute < 0 || clock.Minute > 59 {
			return fmt.Errorf("schedule time must be between 00:00 and 23:59")
		}
	}
	if opts.LogFile == "" || !filepath.IsAbs(opts.LogFile) {
		return fmt.Errorf("log file path must be absolute")
	}
	return nil
}

func launchdLabel(name string) string { return "com.gustmrg.devlog." + name }

func launchdDefinition(opts Options) string {
	var args strings.Builder
	for _, arg := range append([]string{opts.Executable}, opts.Args...) {
		fmt.Fprintf(&args, "    <string>%s</string>\n", html.EscapeString(arg))
	}
	var intervals strings.Builder
	for _, clock := range opts.Times {
		if !opts.Weekdays {
			fmt.Fprintf(&intervals, "    <dict>\n      <key>Hour</key><integer>%d</integer>\n      <key>Minute</key><integer>%d</integer>\n    </dict>\n", clock.Hour, clock.Minute)
			continue
		}
		for weekday := 1; weekday <= 5; weekday++ {
			fmt.Fprintf(&intervals, "    <dict>\n      <key>Weekday</key><integer>%d</integer>\n      <key>Hour</key><integer>%d</integer>\n      <key>Minute</key><integer>%d</integer>\n    </dict>\n", weekday, clock.Hour, clock.Minute)
		}
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>StartCalendarInterval</key>
  <array>
%s  </array>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, launchdLabel(opts.Name), args.String(), intervals.String(), html.EscapeString(opts.LogFile), html.EscapeString(opts.LogFile))
}

func launchdPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel(name)+".plist"), nil
}

func installLaunchd(opts Options) (string, error) {
	path, err := launchdPath(opts.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.WriteFile(path, []byte(launchdDefinition(opts)), 0644); err != nil {
		return "", err
	}
	if output, err := exec.Command("launchctl", "load", path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("could not load launchd job: %s", commandOutput(output, err))
	}
	return path, nil
}

func cronBegin(name string) string { return "# BEGIN devlog " + name }
func cronEnd(name string) string   { return "# END devlog " + name }

func cronDefinition(opts Options) string {
	parts := []string{shellQuote(opts.Executable)}
	for _, arg := range opts.Args {
		parts = append(parts, shellQuote(arg))
	}
	days := "*"
	if opts.Weekdays {
		days = "1-5"
	}
	lines := make([]string, 0, len(opts.Times))
	for _, clock := range opts.Times {
		lines = append(lines, fmt.Sprintf("%d %d * * %s %s >> %s 2>&1", clock.Minute, clock.Hour, days, strings.Join(parts, " "), shellQuote(opts.LogFile)))
	}
	return strings.Join(lines, "\n")
}

func installCron(opts Options) (string, error) {
	current, err := readCrontab()
	if err != nil {
		return "", err
	}
	updated := strings.TrimSpace(removeManagedCronBlock(current, opts.Name))
	if updated != "" {
		updated += "\n\n"
	}
	updated += cronBegin(opts.Name) + "\n" + cronDefinition(opts) + "\n" + cronEnd(opts.Name) + "\n"
	if err := writeCrontab(updated); err != nil {
		return "", err
	}
	return "user crontab", nil
}

func readCrontab() (string, error) {
	output, err := exec.Command("crontab", "-l").CombinedOutput()
	if err == nil {
		return string(output), nil
	}
	if strings.Contains(strings.ToLower(string(output)), "no crontab") {
		return "", nil
	}
	return "", fmt.Errorf("could not read crontab: %s", commandOutput(output, err))
}

func writeCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not update crontab: %s", commandOutput(output, err))
	}
	return nil
}

func commandOutput(output []byte, err error) string {
	if message := strings.TrimSpace(string(output)); message != "" {
		return message
	}
	return err.Error()
}

func managedCronBlock(content, name string) string {
	begin, end := cronBegin(name), cronEnd(name)
	start := strings.Index(content, begin+"\n")
	if start < 0 {
		return ""
	}
	rest := content[start:]
	stop := strings.Index(rest, end)
	if stop < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:stop+len(end)])
}

func removeManagedCronBlock(content, name string) string {
	block := managedCronBlock(content, name)
	if block == "" {
		return content
	}
	return strings.TrimSpace(strings.Replace(content, block, "", 1)) + "\n"
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_@%+=:,./-", r))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func taskName(name string) string { return "DevLog " + name }

// taskDefinition is a Task Scheduler definition that runs the job for the
// current user while they are logged on, catching up on runs missed while the
// computer was off or asleep.
func taskDefinition(opts Options) string {
	var triggers strings.Builder
	for _, clock := range opts.Times {
		schedule := "<ScheduleByDay><DaysInterval>1</DaysInterval></ScheduleByDay>"
		if opts.Weekdays {
			schedule = "<ScheduleByWeek><DaysOfWeek><Monday /><Tuesday /><Wednesday /><Thursday /><Friday /></DaysOfWeek><WeeksInterval>1</WeeksInterval></ScheduleByWeek>"
		}
		fmt.Fprintf(&triggers, "    <CalendarTrigger>\n      <StartBoundary>2026-01-01T%02d:%02d:00</StartBoundary>\n      <Enabled>true</Enabled>\n      %s\n    </CalendarTrigger>\n", clock.Hour, clock.Minute, schedule)
	}
	arguments := make([]string, 0, len(opts.Args)+2)
	for _, arg := range append(append([]string(nil), opts.Args...), "--log-file", opts.LogFile) {
		arguments = append(arguments, windowsQuote(arg))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>%s</Description>
  </RegistrationInfo>
  <Triggers>
%s  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <ExecutionTimeLimit>PT30M</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>%s</Arguments>
    </Exec>
  </Actions>
</Task>
`, html.EscapeString(taskName(opts.Name)), triggers.String(), html.EscapeString(opts.Executable), html.EscapeString(strings.Join(arguments, " ")))
}

func installTask(opts Options) (string, error) {
	file, err := os.CreateTemp("", "devlog-task-*.xml")
	if err != nil {
		return "", err
	}
	path := file.Name()
	defer os.Remove(path)
	_, writeErr := file.Write(utf16LE(taskDefinition(opts)))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return "", fmt.Errorf("could not write the task definition: %v", writeErr)
	}
	if output, err := exec.Command("schtasks", "/Create", "/TN", taskName(opts.Name), "/XML", path, "/F").CombinedOutput(); err != nil {
		return "", fmt.Errorf("could not create the scheduled task: %s", commandOutput(output, err))
	}
	return "Task Scheduler task " + strconv.Quote(taskName(opts.Name)), nil
}

// utf16LE encodes text as UTF-16 little endian with a byte order mark, the
// encoding schtasks expects for task XML.
func utf16LE(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, 2, 2+2*len(units))
	data[0], data[1] = 0xFF, 0xFE
	for _, unit := range units {
		data = append(data, byte(unit), byte(unit>>8))
	}
	return data
}

// windowsQuote quotes one command-line argument the way Windows programs
// split their command line.
func windowsQuote(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	var quoted strings.Builder
	quoted.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			quoted.WriteString(strings.Repeat(`\`, 2*backslashes+1))
			quoted.WriteRune(r)
			backslashes = 0
		default:
			quoted.WriteString(strings.Repeat(`\`, backslashes))
			quoted.WriteRune(r)
			backslashes = 0
		}
	}
	quoted.WriteString(strings.Repeat(`\`, 2*backslashes))
	quoted.WriteByte('"')
	return quoted.String()
}
