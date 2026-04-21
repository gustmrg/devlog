package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
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