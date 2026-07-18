package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"devlog/internal/domain"
)

type Generator interface {
	Generate(context.Context, string, []domain.Activity) (string, error)
}
type Deterministic struct{}

func (Deterministic) Generate(_ context.Context, _ string, activities []domain.Activity) (string, error) {
	var b strings.Builder
	for _, a := range activities {
		fmt.Fprintf(&b, "- %s\n", a.Description)
	}
	return strings.TrimSpace(b.String()), nil
}

type OpenAICompatible struct {
	BaseURL, APIKey, Model, Language, Style string
	Client                                  *http.Client
}

func (g OpenAICompatible) Generate(ctx context.Context, date string, activities []domain.Activity) (string, error) {
	if g.Client == nil {
		g.Client = &http.Client{Timeout: 30 * time.Second}
	}
	language := g.Language
	if language == "" {
		language = "pt-BR"
	}
	style := g.Style
	if style == "" {
		style = "concise"
	}
	prompt := fmt.Sprintf("Create a %s daily development summary for %s in %s using only these verified activities. Do not invent facts:\n", style, date, language)
	for _, a := range activities {
		prompt += fmt.Sprintf("- [%s confidence] %s\n", a.Confidence, a.Description)
	}
	body, _ := json.Marshal(map[string]any{"model": g.Model, "messages": []map[string]string{{"role": "user", "content": prompt}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(g.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.APIKey)
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("LLM returned %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
