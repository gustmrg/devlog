package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"devlog/internal/mcpserver"

	"github.com/spf13/cobra"
)

const (
	mcpServiceName    = "devlog-mcp.service"
	mcpDefaultAddress = "127.0.0.1:8766"
)

// currentExecutable is replaced in tests, whose binary lives in a go-build
// directory that serviceExecutable refuses.
var currentExecutable = os.Executable

// runSystemCommand runs a service manager command; tests replace it.
var runSystemCommand = func(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	return string(output), err
}

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Run the HTTP MCP server as a systemd user service (Linux)",
	Long: `Install devlog-mcp.service as a systemd user service that runs
"devlog mcp --http ADDRESS" from this executable, start it now, and start it
again on every login and after a crash. Running it again replaces the service
and restarts it.

The bearer token file is created if needed. Without lingering, systemd stops
user services when your last session ends; run "loginctl enable-linger" to
keep the server running while you are logged out.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		address, _ := cmd.Flags().GetString("http")
		tokenFile, _ := cmd.Flags().GetString("token-file")
		return installMCPService(address, tokenFile)
	},
}

// installMCPService installs and starts devlog-mcp.service on address, using
// tokenFile or the default token file, and reports what it did.
func installMCPService(address, tokenFile string) error {
	if err := requireSystemd(); err != nil {
		return err
	}
	if err := validateListenAddress(address); err != nil {
		return err
	}
	executable, err := serviceExecutable()
	if err != nil {
		return err
	}
	explicit := tokenFile != ""
	if tokenFile == "" {
		if tokenFile, err = defaultMCPTokenPath(); err != nil {
			return err
		}
	}
	if tokenFile, err = filepath.Abs(tokenFile); err != nil {
		return fmt.Errorf("could not resolve the token file path: %w", err)
	}
	// The service reads the token file, never DEVLOG_MCP_TOKEN, so the file
	// must hold a valid token before the service starts.
	if _, err := mcpTokenFile(tokenFile, explicit); err != nil {
		return err
	}
	unitPath, err := mcpUnitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return fmt.Errorf("could not create the systemd user unit directory: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(mcpUnit(executable, address, tokenFile)), 0o644); err != nil {
		return fmt.Errorf("could not write %s: %w", unitPath, err)
	}
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", mcpServiceName},
		{"--user", "restart", mcpServiceName},
	} {
		if err := systemctl(args...); err != nil {
			return err
		}
	}
	fmt.Printf("Installed and started %s\n", unitPath)
	fmt.Printf("Serving http://%s%s\n", address, mcpserver.EndpointPath)
	fmt.Printf("Bearer token in %s\n", tokenFile)
	if !lingerEnabled() {
		fmt.Println(`Note: lingering is off or unknown, so the service stops when you log out; run "loginctl enable-linger" to keep it running.`)
	}
	return nil
}

var mcpUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove the systemd user service (Linux)",
	Long:  "Stop, disable, and remove devlog-mcp.service. The bearer token file and DevLog data are kept.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireSystemd(); err != nil {
			return err
		}
		unitPath, err := mcpUnitPath()
		if err != nil {
			return err
		}
		if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
			fmt.Printf("%s is not installed.\n", mcpServiceName)
			return nil
		}
		if err := systemctl("--user", "disable", "--now", mcpServiceName); err != nil {
			return err
		}
		if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("could not remove %s: %w", unitPath, err)
		}
		if err := systemctl("--user", "daemon-reload"); err != nil {
			return err
		}
		fmt.Printf("Stopped and removed %s\n", unitPath)
		return nil
	},
}

func systemctl(args ...string) error {
	output, err := runSystemCommand("systemctl", args...)
	if err != nil {
		message := fmt.Sprintf("`systemctl %s` failed", strings.Join(args, " "))
		if detail := strings.TrimSpace(output); detail != "" {
			message += ": " + detail
		}
		return errors.New(message)
	}
	return nil
}

func lingerEnabled() bool {
	current, err := user.Current()
	if err != nil {
		return false
	}
	output, err := runSystemCommand("loginctl", "show-user", current.Username, "--property=Linger", "--value")
	return err == nil && strings.TrimSpace(output) == "yes"
}

func requireSystemd() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("the MCP service is only supported on Linux with systemd; run `devlog mcp --http ADDRESS` under your own service manager instead")
	}
	return nil
}

func validateListenAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		var number int
		number, err = strconv.Atoi(port)
		if err == nil && (number <= 0 || number > 65535) {
			err = errors.New("port out of range")
		}
	}
	if err != nil || host == "" {
		return fmt.Errorf("invalid --http address %q: use HOST:PORT, such as %s", address, mcpDefaultAddress)
	}
	return nil
}

func serviceExecutable() (string, error) {
	executable, err := currentExecutable()
	if err == nil {
		executable, err = filepath.EvalSymlinks(executable)
	}
	if err != nil {
		return "", fmt.Errorf("could not determine the current executable: %w", err)
	}
	if strings.Contains(executable, string(filepath.Separator)+"go-build") {
		return "", fmt.Errorf("run `devlog mcp install` from an installed executable, not `go run`; the service would point at a temporary build")
	}
	return executable, nil
}

func mcpUnitPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not find the home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "systemd", "user", mcpServiceName), nil
}

func mcpUnit(executable, address, tokenFile string) string {
	var unit strings.Builder
	unit.WriteString("# Created by `devlog mcp install`; remove with `devlog mcp uninstall`.\n")
	unit.WriteString("[Unit]\nDescription=DevLog MCP server\n\n")
	unit.WriteString("[Service]\n")
	unit.WriteString("ExecStart=" + strings.Join([]string{
		systemdQuote(executable), "mcp", "--http", systemdQuote(address), "--token-file", systemdQuote(tokenFile),
	}, " ") + "\n")
	unit.WriteString("Restart=on-failure\nRestartSec=5\n\n")
	unit.WriteString("[Install]\nWantedBy=default.target\n")
	return unit.String()
}

// systemdQuote quotes one unit-file word: specifiers (%) and variable
// expansion ($) are escaped, and words with spaces or quotes are double-quoted.
func systemdQuote(value string) string {
	escaped := strings.NewReplacer("%", "%%", "$", "$$").Replace(value)
	if !strings.ContainsAny(escaped, " \t\"'\\;") {
		return escaped
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(escaped) + `"`
}

func init() {
	mcpCmd.AddCommand(mcpInstallCmd, mcpUninstallCmd)
	mcpInstallCmd.Flags().String("http", mcpDefaultAddress, "Address the service listens on")
	mcpInstallCmd.Flags().String("token-file", "", "Bearer token file (default ~/.devlog/mcp-token)")
}
