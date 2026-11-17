package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"devlog/internal/mcpserver"
	"devlog/internal/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const remoteRefreshTimeout = time.Minute

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Serve collected activity over the Model Context Protocol",
	Long: `Serve DevLog's collected activity to AI agents over the Model Context
Protocol, so an agent can turn a day's work into timesheet descriptions.

Tools:
  get_activity  the entries of a day or range, grouped tickets, and when each
                device last collected
  add_entry     record work the user describes, such as a meeting

Without --http the server speaks MCP over stdin and stdout, for clients that
start it as a local process. With --http it serves Streamable HTTP at /mcp on
the given address and requires every request to carry
"Authorization: Bearer <token>". The token comes from DEVLOG_MCP_TOKEN or from
the token file, which is created with a random token on first use.

When remote synchronization is configured, reads first pull the other
devices' activity, at most once a minute, and added entries are pushed.`,
	Example: `  devlog mcp
  devlog mcp --http 127.0.0.1:8766`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		listen, _ := cmd.Flags().GetString("http")
		tokenFile, _ := cmd.Flags().GetString("token-file")
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		server, err := newMCPServer()
		if err != nil {
			return err
		}
		if listen == "" {
			if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("MCP server stopped: %w", err)
			}
			return nil
		}
		token, err := mcpToken(tokenFile, cmd.Flags().Changed("token-file"))
		if err != nil {
			return err
		}
		return mcpserver.ServeHTTP(ctx, listen, token, server, os.Stderr)
	},
}

func newMCPServer() (*mcp.Server, error) {
	repo, err := store.OpenRepository()
	if err != nil {
		return nil, fmt.Errorf("could not open DevLog data: %w", err)
	}
	loc, err := collectLocation()
	if err != nil {
		return nil, err
	}
	tickets, err := ticketPattern()
	if err != nil {
		return nil, err
	}
	server := &mcpserver.Server{
		Repository:     repo,
		Location:       loc,
		DeviceName:     deviceName(),
		DefaultProject: viper.GetString("defaults.project"),
		Tickets:        tickets,
		Version:        versionInfo,
	}
	if viper.GetBool("remote.enabled") {
		server.Refresh = func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, remoteRefreshTimeout)
			defer cancel()
			return configuredRemote(repo).Sync(ctx)
		}
	}
	return server.MCP(), nil
}

// mcpToken returns the bearer token for the HTTP transport, creating the
// default token file on first use. An explicitly named file must exist.
func mcpToken(path string, explicit bool) (string, error) {
	if value := strings.TrimSpace(os.Getenv("DEVLOG_MCP_TOKEN")); value != "" {
		if len(value) < mcpserver.MinTokenLength {
			return "", fmt.Errorf("DEVLOG_MCP_TOKEN must be at least %d characters", mcpserver.MinTokenLength)
		}
		return value, nil
	}
	return mcpTokenFile(path, explicit)
}

// mcpTokenFile returns the token saved in path, or in the default token file,
// creating the default file with a random token when it does not exist yet.
func mcpTokenFile(path string, explicit bool) (string, error) {
	if path == "" {
		var err error
		if path, err = defaultMCPTokenPath(); err != nil {
			return "", err
		}
	}
	body, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(body))
		if len(token) < mcpserver.MinTokenLength {
			return "", fmt.Errorf("the token in %s must be at least %d characters", path, mcpserver.MinTokenLength)
		}
		return token, nil
	}
	if !errors.Is(err, os.ErrNotExist) || explicit {
		return "", fmt.Errorf("could not read MCP token file %s: %w", path, err)
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("could not generate an MCP token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("could not create the MCP token directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("could not create MCP token file %s: %w", path, err)
	}
	_, writeErr := file.WriteString(token + "\n")
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("could not write MCP token file %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Generated a bearer token in %s\n", path)
	return token, nil
}

func defaultMCPTokenPath() (string, error) {
	root, err := store.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "mcp-token"), nil
}

func init() {
	RootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().String("http", "", "Serve Streamable HTTP on this address, such as 127.0.0.1:8766")
	mcpCmd.Flags().String("token-file", "", "Bearer token file (default ~/.devlog/mcp-token)")
}
