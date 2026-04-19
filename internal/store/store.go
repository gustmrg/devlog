package store

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

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

	viper.Set("defaults.project", "")
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

	return home + "/.devlog", nil
}