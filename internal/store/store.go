package store

import (
	"encoding/json"
	"errors"
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
	Id          string     `json:"id"`
	Project     string     `json:"project"`
	Description string     `json:"description"`
	Tags        []string   `json:"tags"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt,omitempty"`
	DeletedAt   *time.Time `json:"deletedAt,omitempty"`
	// Source identifies the external origin of an entry (e.g. "github:commit:<sha>"),
	// used to keep repeated syncs idempotent. Empty for manually added entries.
	Source string `json:"source,omitempty"`
}

type Summary struct {
	ID          string // "2026-04-21", used as filename
	Date        time.Time
	Projects    []ProjectGroup
	Style       string
	AIGenerated bool
	GeneratedAt time.Time
	DeviceID    string
	Content     string
}

type SummaryMeta struct {
	Date        string    `yaml:"date"`
	Style       string    `yaml:"style"`
	Projects    string    `yaml:"projects"`
	AIGenerated bool      `yaml:"aiGenerated,omitempty"`
	GeneratedAt time.Time `yaml:"generatedAt,omitempty"`
	DeviceID    string    `yaml:"deviceId,omitempty"`
}

type ProjectGroup struct {
	Name    string
	Entries []Entry
}

func LoadDailyLog(filePath string) (DailyLog, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return DailyLog{}, nil
	}
	if err != nil {
		return DailyLog{}, fmt.Errorf("could not read legacy log %s: %w", filePath, err)
	}
	var log DailyLog
	if err := json.Unmarshal(data, &log); err != nil {
		return DailyLog{}, fmt.Errorf("could not parse legacy log %s: %w", filePath, err)
	}
	return log, nil
}

func SaveDailyLog(filePath string, log DailyLog) error {
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode legacy log: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("could not write legacy log %s: %w", filePath, err)
	}
	return nil
}

func LoadSummary(filePath string) (Summary, error) {
	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return Summary{}, nil
	}
	if err != nil {
		return Summary{}, fmt.Errorf("could not read summary %s: %w", filePath, err)
	}

	return ParseSummary(data)
}

func ParseSummary(data []byte) (Summary, error) {
	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) < 3 {
		return Summary{}, fmt.Errorf("invalid summary file: missing frontmatter")
	}

	var meta SummaryMeta
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return Summary{}, fmt.Errorf("could not parse summary frontmatter: %w", err)
	}

	date, err := time.Parse("2006-01-02", meta.Date)
	if err != nil {
		return Summary{}, fmt.Errorf("summary has an invalid date %q: expected YYYY-MM-DD", meta.Date)
	}

	var projects []ProjectGroup
	for p := range strings.SplitSeq(meta.Projects, ",") {
		if name := strings.TrimSpace(p); name != "" {
			projects = append(projects, ProjectGroup{Name: name})
		}
	}

	return Summary{
		ID:          meta.Date,
		Date:        date,
		Style:       meta.Style,
		Projects:    projects,
		AIGenerated: meta.AIGenerated,
		GeneratedAt: meta.GeneratedAt,
		DeviceID:    meta.DeviceID,
		Content:     strings.TrimSpace(parts[2]),
	}, nil
}

func SaveSummary(filePath string, summary Summary) error {
	return saveSummaryAtomic(filePath, summary)
}

func saveSummaryAtomic(filePath string, summary Summary) error {
	content := EncodeSummary(summary)
	if err := atomicWrite(filePath, content, 0644); err != nil {
		return fmt.Errorf("could not write summary %s: %w", filePath, err)
	}
	return nil
}

func EncodeSummary(summary Summary) []byte {
	projectNames := make([]string, len(summary.Projects))
	for i, p := range summary.Projects {
		projectNames[i] = p.Name
	}
	content := fmt.Sprintf("---\ndate: %s\nstyle: %s\nprojects: %s\naiGenerated: %t\ngeneratedAt: %s\ndeviceId: %s\n---\n%s\n",
		summary.Date.Format("2006-01-02"),
		summary.Style,
		strings.Join(projectNames, ", "),
		summary.AIGenerated,
		summary.GeneratedAt.UTC().Format(time.RFC3339Nano),
		summary.DeviceID,
		summary.Content,
	)
	return []byte(content)
}

func marshalEntry(entry Entry) ([]byte, error) {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// InitResult describes whether initialization created a new configuration.
type InitResult struct {
	ConfigFile string
	Created    bool
}

// Initialize creates DevLog's default configuration without overwriting an
// existing one.
func Initialize() (InitResult, error) {
	path, err := ConfigPath()
	if err != nil {
		return InitResult{}, err
	}
	result := InitResult{ConfigFile: filepath.Join(path, "config.json")}

	if err := os.MkdirAll(path, 0755); err != nil {
		return InitResult{}, fmt.Errorf("could not create application directory %s: %w", path, err)
	}

	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("json")

	viper.Set("defaults.project", "default")
	viper.Set("defaults.style", "concise")
	viper.Set("defaults.language", "pt-BR")
	viper.Set("storage.path", "")
	viper.Set("llm.enabled", false)
	viper.Set("llm.provider", "openrouter")
	viper.Set("llm.model", "openai/gpt-oss-120b:free")
	viper.Set("llm.apiKeyEnvVar", "OPENROUTER_API_KEY")
	viper.Set("github.username", "")
	viper.Set("github.tokenEnvVar", "GITHUB_TOKEN")
	viper.Set("remote.enabled", false)
	viper.Set("remote.url", "")
	viper.Set("remote.branch", "main")

	if err := viper.SafeWriteConfig(); err != nil {
		var alreadyExists viper.ConfigFileAlreadyExistsError
		if errors.As(err, &alreadyExists) {
			return result, nil
		}
		return InitResult{}, fmt.Errorf("could not write config file %s: %w", result.ConfigFile, err)
	}

	result.Created = true
	return result, nil
}

// Init preserves the original error-only API for callers that do not need to
// distinguish a new configuration from an existing one.
func Init() error {
	_, err := Initialize()
	return err
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find the home directory: %w", err)
	}

	return filepath.Join(home, ".devlog"), nil
}
