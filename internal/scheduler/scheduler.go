// Package scheduler installs and manages the platform-native daily sync job.
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
)

const (
	launchdLabel = "com.gustmrg.devlog.sync"
	cronBegin    = "# BEGIN devlog sync"
	cronEnd      = "# END devlog sync"
)

type Options struct {
	Executable string
	Hour       int
	Minute     int
	Polish     bool
	Remote     bool
	EnvFile    string
	LogFile    string
}

func ParseTime(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time %q: expected HH:MM", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("invalid hour in %q", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid minute in %q", value)
	}
	return hour, minute, nil
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
	default:
		return "", fmt.Errorf("automatic sync is not supported on %s", runtime.GOOS)
	}
}

func Show() (string, bool, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchdPath()
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
		block := managedCronBlock(current)
		return block, block != "", nil
	default:
		return "", false, fmt.Errorf("automatic sync is not supported on %s", runtime.GOOS)
	}
}

func Remove() (bool, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := launchdPath()
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
		if managedCronBlock(current) == "" {
			return false, nil
		}
		return true, writeCrontab(removeManagedCronBlock(current))
	default:
		return false, fmt.Errorf("automatic sync is not supported on %s", runtime.GOOS)
	}
}

func validate(opts Options) error {
	if opts.Executable == "" || !filepath.IsAbs(opts.Executable) {
		return fmt.Errorf("devlog executable path must be absolute")
	}
	if opts.Hour < 0 || opts.Hour > 23 || opts.Minute < 0 || opts.Minute > 59 {
		return fmt.Errorf("invalid schedule time")
	}
	if opts.LogFile == "" || !filepath.IsAbs(opts.LogFile) {
		return fmt.Errorf("log file path must be absolute")
	}
	if opts.EnvFile != "" && !filepath.IsAbs(opts.EnvFile) {
		return fmt.Errorf("environment file path must be absolute")
	}
	return nil
}

func arguments(opts Options) []string {
	args := []string{"sync"}
	if opts.Polish {
		args = append(args, "--polish")
	}
	if opts.Remote {
		args = append(args, "--remote")
	}
	if opts.EnvFile != "" {
		args = append(args, "--env-file", opts.EnvFile)
	}
	return args
}

func launchdDefinition(opts Options) string {
	var args strings.Builder
	for _, arg := range append([]string{opts.Executable}, arguments(opts)...) {
		fmt.Fprintf(&args, "    <string>%s</string>\n", html.EscapeString(arg))
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
  <dict>
    <key>Hour</key><integer>%d</integer>
    <key>Minute</key><integer>%d</integer>
  </dict>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, launchdLabel, args.String(), opts.Hour, opts.Minute, html.EscapeString(opts.LogFile), html.EscapeString(opts.LogFile))
}

func launchdPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func installLaunchd(opts Options) (string, error) {
	path, err := launchdPath()
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
		return "", fmt.Errorf("launchctl load failed: %s", strings.TrimSpace(string(output)))
	}
	return path, nil
}

func cronDefinition(opts Options) string {
	parts := []string{shellQuote(opts.Executable)}
	for _, arg := range arguments(opts) {
		parts = append(parts, shellQuote(arg))
	}
	return fmt.Sprintf("%d %d * * * %s >> %s 2>&1", opts.Minute, opts.Hour, strings.Join(parts, " "), shellQuote(opts.LogFile))
}

func installCron(opts Options) (string, error) {
	current, err := readCrontab()
	if err != nil {
		return "", err
	}
	updated := strings.TrimSpace(removeManagedCronBlock(current))
	if updated != "" {
		updated += "\n\n"
	}
	updated += cronBegin + "\n" + cronDefinition(opts) + "\n" + cronEnd + "\n"
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
	return "", fmt.Errorf("could not read crontab: %s", strings.TrimSpace(string(output)))
}

func writeCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not update crontab: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func managedCronBlock(content string) string {
	start := strings.Index(content, cronBegin)
	if start < 0 {
		return ""
	}
	rest := content[start:]
	end := strings.Index(rest, cronEnd)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end+len(cronEnd)])
}

func removeManagedCronBlock(content string) string {
	block := managedCronBlock(content)
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
