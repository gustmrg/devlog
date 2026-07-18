package cmd

import (
	"runtime"
	"strings"
	"testing"
)

func TestServiceDefinitionUsesCurrentExecutable(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported test platform")
	}
	path, content, command, err := serviceFile()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" || len(command) == 0 || !strings.Contains(content, "agent") || !strings.Contains(content, "run") {
		t.Fatalf("path=%q command=%v content=%q", path, command, content)
	}
}
