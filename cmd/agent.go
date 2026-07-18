package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"text/template"
	"time"

	"devlog/internal/agent"
	"devlog/internal/config"
	"devlog/internal/syncapi"
	"github.com/spf13/cobra"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Manage the local collection agent"}
	cmd.AddCommand(newAgentRunCmd(), newAgentInstallCmd(), newAgentUninstallCmd(), newAgentStatusCmd())
	return cmd
}
func loadAgent(interval time.Duration) (*agent.Agent, func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load(filepath.Join(home, ".devlog", "config.json"))
	if err != nil {
		return nil, nil, err
	}
	db, err := agent.Open(home)
	if err != nil {
		return nil, nil, err
	}
	_, credentialsPath := agent.Paths(home)
	credentials, err := syncapi.LoadCredentials(credentialsPath)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("device is not connected: %w", err)
	}
	return &agent.Agent{Config: cfg, DB: db, Credentials: credentials, Interval: interval}, func() { db.Close() }, nil
}
func newAgentRunCmd() *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{Use: "run", Short: "Run collection and synchronization in the foreground", RunE: func(cmd *cobra.Command, args []string) error {
		a, closeFn, err := loadAgent(interval)
		if err != nil {
			return err
		}
		defer closeFn()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = a.Run(ctx)
		if err == context.Canceled {
			return nil
		}
		return err
	}}
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Minute, "Collection interval")
	return cmd
}

const launchdTemplate = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>com.devlog.agent</string><key>ProgramArguments</key><array><string>{{.Executable}}</string><string>agent</string><string>run</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>StandardOutPath</key><string>{{.Home}}/.devlog/agent.log</string><key>StandardErrorPath</key><string>{{.Home}}/.devlog/agent.log</string></dict></plist>`
const systemdTemplate = `[Unit]
Description=DevLog local collection agent
After=network-online.target
[Service]
ExecStart={{.Executable}} agent run
Restart=on-failure
RestartSec=10
[Install]
WantedBy=default.target
`

func serviceFile() (string, string, []string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", "", nil, err
	}
	data := struct{ Executable, Home string }{exe, home}
	var path, tpl string
	var command []string
	switch runtime.GOOS {
	case "darwin":
		path = filepath.Join(home, "Library", "LaunchAgents", "com.devlog.agent.plist")
		tpl = launchdTemplate
		command = []string{"launchctl", "bootstrap", "gui/" + fmt.Sprint(os.Getuid()), path}
	case "linux":
		path = filepath.Join(home, ".config", "systemd", "user", "devlog-agent.service")
		tpl = systemdTemplate
		command = []string{"systemctl", "--user", "enable", "--now", "devlog-agent"}
	default:
		return "", "", nil, fmt.Errorf("agent installation is unsupported on %s", runtime.GOOS)
	}
	var out stringBuilder
	if err := template.Must(template.New("service").Parse(tpl)).Execute(&out, data); err != nil {
		return "", "", nil, err
	}
	return path, out.String(), command, nil
}

type stringBuilder struct{ b []byte }

func (s *stringBuilder) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }
func (s *stringBuilder) String() string              { return string(s.b) }
func newAgentInstallCmd() *cobra.Command {
	return &cobra.Command{Use: "install", Short: "Install and start the agent as a user service", RunE: func(cmd *cobra.Command, args []string) error {
		path, content, start, err := serviceFile()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
		if runtime.GOOS == "darwin" {
			_ = exec.CommandContext(cmd.Context(), "launchctl", "bootout", "gui/"+fmt.Sprint(os.Getuid()), path).Run()
		} else if err := exec.CommandContext(cmd.Context(), "systemctl", "--user", "daemon-reload").Run(); err != nil {
			return fmt.Errorf("systemd daemon-reload: %w", err)
		}
		run := exec.CommandContext(cmd.Context(), start[0], start[1:]...)
		run.Stdout = cmd.OutOrStdout()
		run.Stderr = cmd.ErrOrStderr()
		if err := run.Run(); err != nil {
			return fmt.Errorf("service file written to %s, but start failed: %w", path, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Agent installed: %s\n", path)
		return nil
	}}
}
func newAgentUninstallCmd() *cobra.Command {
	return &cobra.Command{Use: "uninstall", Short: "Stop and remove the user service", RunE: func(cmd *cobra.Command, args []string) error {
		path, _, _, err := serviceFile()
		if err != nil {
			return err
		}
		if runtime.GOOS == "darwin" {
			_ = exec.CommandContext(cmd.Context(), "launchctl", "bootout", "gui/"+fmt.Sprint(os.Getuid()), path).Run()
		} else {
			_ = exec.CommandContext(cmd.Context(), "systemctl", "--user", "disable", "--now", "devlog-agent").Run()
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Agent uninstalled")
		return nil
	}}
}
func newAgentStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show agent service status", RunE: func(cmd *cobra.Command, args []string) error {
		var run *exec.Cmd
		if runtime.GOOS == "darwin" {
			run = exec.CommandContext(cmd.Context(), "launchctl", "print", "gui/"+fmt.Sprint(os.Getuid())+"/com.devlog.agent")
		} else if runtime.GOOS == "linux" {
			run = exec.CommandContext(cmd.Context(), "systemctl", "--user", "status", "devlog-agent")
		} else {
			return fmt.Errorf("unsupported platform")
		}
		run.Stdout = cmd.OutOrStdout()
		run.Stderr = cmd.ErrOrStderr()
		return run.Run()
	}}
}
