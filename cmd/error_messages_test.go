package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestRootDoesNotPrintUsageForRuntimeErrors(t *testing.T) {
	if !RootCmd.SilenceUsage {
		t.Fatal("root command should suppress usage output when a command returns an error")
	}
}

func TestMissingGitHubAuthenticationMessageIsActionable(t *testing.T) {
	t.Setenv("DEVLOG_TEST_GITHUB_TOKEN", "")
	t.Setenv("PATH", "")

	_, _, err := resolveGitHubToken(context.Background(), "DEVLOG_TEST_GITHUB_TOKEN")
	if err == nil {
		t.Fatal("missing authentication did not return an error")
	}
	message := err.Error()
	for _, expected := range []string{"DEVLOG_TEST_GITHUB_TOKEN", "GitHub CLI"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("error %q does not contain %q", message, expected)
		}
	}
}

func TestInvalidConfigBooleanIncludesValueAndExpectedFormat(t *testing.T) {
	_, err := parseConfigValue("llm.enabled", "yes")
	if err == nil {
		t.Fatal("invalid boolean did not return an error")
	}
	if got, want := err.Error(), `invalid value "yes" for llm.enabled: expected true or false`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}
