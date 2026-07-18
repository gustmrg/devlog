package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Defaults      DefaultsConfig  `json:"defaults"`
	Device        DeviceConfig    `json:"device,omitempty"`
	Server        ServerConfig    `json:"server,omitempty"`
	Discovery     DiscoveryConfig `json:"discovery,omitempty"`
	GitHub        GitHubConfig    `json:"github,omitempty"`
	LLM           LLMConfig       `json:"llm,omitempty"`
	Discord       DiscordConfig   `json:"discord,omitempty"`
	Schedules     ScheduleConfig  `json:"schedules,omitempty"`
	RetentionDays int             `json:"retentionDays,omitempty"`
	Projects      []ProjectConfig `json:"projects,omitempty"`
}

type DefaultsConfig struct {
	Project  string `json:"project"`
	Style    string `json:"style"`
	Language string `json:"language"`
}
type DeviceConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ServerConfig struct {
	URL           string `json:"url,omitempty"`
	AllowInsecure bool   `json:"allowInsecure,omitempty"`
}
type DiscoveryConfig struct {
	Roots    []string `json:"roots"`
	Exclude  []string `json:"exclude,omitempty"`
	MaxDepth int      `json:"maxDepth"`
}
type GitHubConfig struct {
	Enabled     bool   `json:"enabled"`
	TokenEnvVar string `json:"tokenEnvVar"`
	Interval    string `json:"interval"`
}
type LLMConfig struct {
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"baseUrl"`
	Model        string `json:"model"`
	APIKeyEnvVar string `json:"apiKeyEnvVar"`
}
type DiscordConfig struct {
	Enabled     bool   `json:"enabled"`
	TokenEnvVar string `json:"tokenEnvVar"`
	GuildID     string `json:"guildId,omitempty"`
	ChannelID   string `json:"channelId,omitempty"`
	UserID      string `json:"userId,omitempty"`
}
type ScheduleConfig struct {
	Timezone          string `json:"timezone"`
	CorrelateInterval string `json:"correlateInterval"`
	FinalizeAt        string `json:"finalizeAt"`
	SummaryAt         string `json:"summaryAt"`
}
type ProjectConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Remote  string `json:"remote,omitempty"`
	Enabled bool   `json:"enabled"`
}

func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Defaults:      DefaultsConfig{Project: "default", Style: "concise", Language: "pt-BR"},
		Discovery:     DiscoveryConfig{Roots: []string{filepath.Join(home, "Dev")}, MaxDepth: 3},
		GitHub:        GitHubConfig{Interval: "15m", TokenEnvVar: "DEVLOG_GITHUB_TOKEN"},
		LLM:           LLMConfig{BaseURL: "https://openrouter.ai/api/v1", Model: "openai/gpt-oss-120b:free", APIKeyEnvVar: "DEVLOG_LLM_API_KEY"},
		Discord:       DiscordConfig{TokenEnvVar: "DEVLOG_DISCORD_BOT_TOKEN"},
		Schedules:     ScheduleConfig{Timezone: "America/Fortaleza", CorrelateInterval: "30m", FinalizeAt: "17:45", SummaryAt: "18:00"},
		RetentionDays: 30,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, cfg.Validate()
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (c Config) Validate() error {
	if c.Discovery.MaxDepth < 0 {
		return errors.New("discovery.maxDepth must not be negative")
	}
	if c.RetentionDays < 0 {
		return errors.New("retentionDays must not be negative")
	}
	if _, err := time.LoadLocation(c.Schedules.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	for name, value := range map[string]string{"correlateInterval": c.Schedules.CorrelateInterval, "github.interval": c.GitHub.Interval} {
		if value != "" {
			if _, err := time.ParseDuration(value); err != nil {
				return fmt.Errorf("invalid %s: %w", name, err)
			}
		}
	}
	return nil
}

func Expand(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
