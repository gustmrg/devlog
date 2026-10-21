package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// TestRunnableCommandsValidateArguments guards against commands that ignore
// stray arguments: "devlog sync schedule now" used to install a job instead of
// reporting that "now" is not a subcommand.
func TestRunnableCommandsValidateArguments(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if command.Runnable() && command.Args == nil {
			t.Errorf("%s accepts any arguments; set Args, such as cobra.NoArgs", command.CommandPath())
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(RootCmd)
}

func TestUnknownSubcommandIsRejected(t *testing.T) {
	RootCmd.SetArgs([]string{"sync", "schedule", "now"})
	t.Cleanup(func() { RootCmd.SetArgs(nil) })
	err := RootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "now"`) {
		t.Fatalf("error = %v, want an unknown command error", err)
	}
}
