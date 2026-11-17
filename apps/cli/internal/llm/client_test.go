package llm

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestDisabledMessageUsesConfigCommand(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	_, err := NewFromConfig()
	if err == nil {
		t.Fatal("disabled LLM configuration did not return an error")
	}
	if !strings.Contains(err.Error(), "devlog config set llm.enabled true") {
		t.Fatalf("error is not actionable: %q", err)
	}
}

func TestMissingAPIKeyMessageNamesEnvironmentVariable(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("llm.enabled", true)
	viper.Set("llm.provider", "openai")
	viper.Set("llm.model", "test-model")
	viper.Set("llm.apiKeyEnvVar", "DEVLOG_TEST_LLM_KEY")
	t.Setenv("DEVLOG_TEST_LLM_KEY", "")

	_, err := NewFromConfig()
	if err == nil {
		t.Fatal("missing API key did not return an error")
	}
	if !strings.Contains(err.Error(), "DEVLOG_TEST_LLM_KEY") {
		t.Fatalf("error does not name the required variable: %q", err)
	}
}
