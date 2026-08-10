// Package templates embeds the style prompt templates used for
// LLM-generated entries and summaries.
package templates

import (
	"embed"
	"fmt"
)

//go:embed *.md
var files embed.FS

// Get returns the prompt template for the given style
// (concise, detailed, formal, impersonal).
func Get(style string) (string, error) {
	data, err := files.ReadFile(style + ".md")
	if err != nil {
		return "", fmt.Errorf("unknown style %q (available: concise, detailed, formal, impersonal)", style)
	}
	return string(data), nil
}
