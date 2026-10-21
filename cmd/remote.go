package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"devlog/internal/gitremote"
	"devlog/internal/store"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Synchronize DevLog data through a private Git repository",
}

var remoteInitCmd = &cobra.Command{
	Use:   "init <repository-url>",
	Short: "Connect local DevLog data to a Git repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		branch, _ := cmd.Flags().GetString("branch")
		if err := initRemote(cmd.Context(), args[0], branch); err != nil {
			return err
		}
		fmt.Printf("Remote synchronization initialized on branch %s.\n", branch)
		return nil
	},
}

// initRemote connects the local data to the repository at url, merging what
// other devices already pushed, and saves the remote settings.
func initRemote(ctx context.Context, url, branch string) error {
	repo, err := store.OpenRepository()
	if err != nil {
		return fmt.Errorf("could not open DevLog data: %w", err)
	}
	if err := gitremote.New(repo, url, branch).Init(ctx); err != nil {
		return fmt.Errorf("could not initialize remote synchronization: %w", err)
	}
	viper.Set("remote.enabled", true)
	viper.Set("remote.url", url)
	viper.Set("remote.branch", branch)
	if err := persistConfig(); err != nil {
		return fmt.Errorf("remote synchronization was initialized, but the configuration could not be saved: %w", err)
	}
	return nil
}

var remoteStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show remote synchronization status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, err := store.OpenRepository()
		if err != nil {
			return fmt.Errorf("could not open DevLog data: %w", err)
		}
		manager := configuredRemote(repo)
		status, err := manager.Status(cmd.Context())
		if err != nil {
			return fmt.Errorf("could not read remote synchronization status: %w", err)
		}
		if !status.Initialized {
			fmt.Println("Remote synchronization is not initialized.")
			return nil
		}
		fmt.Printf("Remote: %s\nBranch: %s\n", status.URL, status.Branch)
		fmt.Printf("Working tree: %s\n", map[bool]string{true: "modified", false: "clean"}[status.Dirty])
		fmt.Printf("Ahead: %d, behind: %d\n", status.Ahead, status.Behind)
		return nil
	},
}

var remoteSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull, merge, and push DevLog data",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, err := store.OpenRepository()
		if err != nil {
			return fmt.Errorf("could not open DevLog data: %w", err)
		}
		if err := configuredRemote(repo).Sync(cmd.Context()); err != nil {
			return fmt.Errorf("could not synchronize remote data: %w", err)
		}
		fmt.Println("Remote synchronization complete.")
		return nil
	},
}

var remoteRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Disconnect the remote without deleting local data",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, err := store.OpenRepository()
		if err != nil {
			return fmt.Errorf("could not open DevLog data: %w", err)
		}
		if err := configuredRemote(repo).Disconnect(cmd.Context()); err != nil {
			return fmt.Errorf("could not disconnect remote synchronization: %w", err)
		}
		viper.Set("remote.enabled", false)
		viper.Set("remote.url", "")
		if err := persistConfig(); err != nil {
			return fmt.Errorf("remote was disconnected, but the configuration could not be saved: %w", err)
		}
		fmt.Println("Remote synchronization disconnected. Local data was kept.")
		return nil
	},
}

func configuredRemote(repo *store.Repository) *gitremote.Manager {
	return gitremote.New(repo, viper.GetString("remote.url"), viper.GetString("remote.branch"))
}

func syncConfiguredRemote(ctx context.Context, repo *store.Repository) error {
	if !viper.GetBool("remote.enabled") {
		return fmt.Errorf("remote synchronization is disabled; run devlog remote init <url>")
	}
	return configuredRemote(repo).Sync(ctx)
}

func persistConfig() error {
	if path := viper.ConfigFileUsed(); path != "" {
		return viper.WriteConfig()
	}
	root, err := store.ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	return viper.WriteConfigAs(filepath.Join(root, "config.json"))
}

func init() {
	RootCmd.AddCommand(remoteCmd)
	remoteCmd.AddCommand(remoteInitCmd, remoteStatusCmd, remoteSyncCmd, remoteRemoveCmd)
	remoteInitCmd.Flags().String("branch", "main", "Remote branch used for DevLog data")
}
