package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

type DailyLog struct {
	Date    string  `json:"date"`
	Entries []Entry `json:"entries"`
}

type Entry struct {
	Id          string    `json:"id"`
	Project     string    `json:"project"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Summary struct {
	ID string // "2026-04-21", used as filename
	Date time.Time
	Projects []ProjectGroup
	Style string
	AIGenerated bool
	Content string
}

type SummaryMeta struct {
    Date     string `yaml:"date"`
    Style    string `yaml:"style"`
    Projects string `yaml:"projects"`
}

type ProjectGroup struct {
	Name string
	Entries []Entry
}

func LoadDailyLog(filePath string) (DailyLog, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return DailyLog{}, nil
	}
	if err != nil {
		return DailyLog{}, fmt.Errorf("error reading log file: %w", err)
	}
	var log DailyLog
	if err := json.Unmarshal(data, &log); err != nil {
		return DailyLog{}, fmt.Errorf("error parsing log file: %w", err)
	}
	return log, nil
}

func SaveDailyLog(filePath string, log DailyLog) error {
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return fmt.Errorf("error encoding log: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("error writing log file: %w", err)
	}
	return nil
}

func LoadSummary(filePath string) (Summary, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return Summary{}, nil
	}
	if err != nil {
		return Summary{}, fmt.Errorf("error reading summary file: %w", err)
	}

	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) < 3 {
		return Summary{}, fmt.Errorf("invalid summary file: missing frontmatter")
	}

	var meta SummaryMeta
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return Summary{}, fmt.Errorf("error parsing summary frontmatter: %w", err)
	}

	date, err := time.Parse("2006-01-02", meta.Date)
	if err != nil {
		return Summary{}, fmt.Errorf("invalid date in summary: %w", err)
	}

	var projects []ProjectGroup
	for p := range strings.SplitSeq(meta.Projects, ",") {
		if name := strings.TrimSpace(p); name != "" {
			projects = append(projects, ProjectGroup{Name: name})
		}
	}

	return Summary{
		ID:       meta.Date,
		Date:     date,
		Style:    meta.Style,
		Projects: projects,
		Content:  strings.TrimSpace(parts[2]),
	}, nil
}

func Init() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(path, 0755); err != nil {
    	return fmt.Errorf("fatal error creating application directory: %w", err)
	}

	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("json")

	viper.Set("defaults.project", "default")
	viper.Set("defaults.style", "concise")
	viper.Set("defaults.language", "pt-BR")
	viper.Set("llm.enabled", false)
	viper.Set("llm.provider", "openrouter")
	viper.Set("llm.model", "openai/gpt-oss-120b:free")
	viper.Set("llm.apiKeyEnvVar", "OPENROUTER_API_KEY")

	if err := viper.SafeWriteConfig(); err != nil {
		return fmt.Errorf("fatal error writing config file: %w", err)
	}

	return nil
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("fatal error could not find home directory: %w", err)
	}

	return filepath.Join(home, ".devlog"), nil
}