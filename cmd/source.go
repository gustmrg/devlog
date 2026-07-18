package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devlog/internal/collector/gitlocal"
	"devlog/internal/config"
	"github.com/spf13/cobra"
)

func newSourceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "Discover and configure collection sources"}
	cmd.AddCommand(sourceDiscoverCmd(), sourceListCmd(), sourceEnableCmd(), sourceDisableCmd())
	return cmd
}
func localConfig() (string, config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", config.Config{}, err
	}
	path := filepath.Join(home, ".devlog", "config.json")
	cfg, err := config.Load(path)
	return path, cfg, err
}
func sourceDiscoverCmd() *cobra.Command {
	return &cobra.Command{Use: "discover [root...]", Short: "Discover Git repositories", RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := localConfig()
		if err != nil {
			return err
		}
		roots := args
		if len(roots) == 0 {
			roots = cfg.Discovery.Roots
		}
		repos, err := gitlocal.Discover(roots, cfg.Discovery.MaxDepth)
		if err != nil {
			return err
		}
		for _, repo := range repos {
			if sourceExcluded(repo, cfg.Discovery.Exclude) {
				continue
			}
			fmt.Fprintln(cmd.OutOrStdout(), repo)
		}
		return nil
	}}
}
func sourceExcluded(repo string, excludes []string) bool {
	repo = filepath.Clean(repo)
	for _, exclude := range excludes {
		value := filepath.Clean(config.Expand(exclude))
		if repo == value || strings.HasPrefix(repo, value+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
func sourceListCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List configured projects", RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := localConfig()
		if err != nil {
			return err
		}
		for _, p := range cfg.Projects {
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-8t %s\n", p.ID, p.Enabled, p.Path)
		}
		return nil
	}}
}
func sourceEnableCmd() *cobra.Command {
	var projectID string
	cmd := &cobra.Command{Use: "enable <repository-path>", Short: "Enable collection for a local repository", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path, cfg, err := localConfig()
		if err != nil {
			return err
		}
		repo, err := filepath.Abs(config.Expand(args[0]))
		if err != nil {
			return err
		}
		remote := readGit(repo, "remote", "get-url", "origin")
		remote = gitlocal.NormalizeRemote(remote)
		id := projectID
		if id == "" {
			id = strings.ToLower(filepath.Base(repo))
		}
		for i := range cfg.Projects {
			if cfg.Projects[i].Path == repo {
				cfg.Projects[i].Enabled = true
				return config.Save(path, cfg)
			}
		}
		cfg.Projects = append(cfg.Projects, config.ProjectConfig{ID: id, Name: filepath.Base(repo), Path: repo, Remote: remote, Enabled: true})
		return config.Save(path, cfg)
	}}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Central project ID (defaults to repository directory name)")
	return cmd
}
func sourceDisableCmd() *cobra.Command {
	return &cobra.Command{Use: "disable <project-id>", Short: "Disable a configured project", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		path, cfg, err := localConfig()
		if err != nil {
			return err
		}
		for i := range cfg.Projects {
			if cfg.Projects[i].ID == args[0] {
				cfg.Projects[i].Enabled = false
				return config.Save(path, cfg)
			}
		}
		return fmt.Errorf("project %s not found", args[0])
	}}
}
func readGit(root string, args ...string) string {
	out, _ := execGit(root, args...)
	return strings.TrimSpace(out)
}
func execGit(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}
