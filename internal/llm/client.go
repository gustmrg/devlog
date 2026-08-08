// Package llm provides a minimal client for OpenAI-compatible chat
// completions endpoints (OpenRouter by default), used to generate
// polished summaries and entries.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/viper"
)

// providerBaseURLs maps known llm.provider values to their API base URL.
var providerBaseURLs = map[string]string{
	"openrouter": "https://openrouter.ai/api/v1",
	"openai":     "https://api.openai.com/v1",
}

// Client calls an OpenAI-compatible chat completions API.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// NewFromConfig builds a client from the llm.* keys in the devlog config.
func NewFromConfig() (*Client, error) {
	if !viper.GetBool("llm.enabled") {
		return nil, fmt.Errorf("LLM support is disabled — set llm.enabled to true in ~/.devlog/config.json")
	}

	baseURL := viper.GetString("llm.baseURL")
	if baseURL == "" {
		provider := viper.GetString("llm.provider")
		known, ok := providerBaseURLs[provider]
		if !ok {
			return nil, fmt.Errorf("unknown llm.provider %q — set llm.baseURL for a custom OpenAI-compatible endpoint", provider)
		}
		baseURL = known
	}

	model := viper.GetString("llm.model")
	if model == "" {
		return nil, fmt.Errorf("llm.model is not set in the devlog config")
	}

	apiKeyEnvVar := viper.GetString("llm.apiKeyEnvVar")
	if apiKeyEnvVar == "" {
		apiKeyEnvVar = "OPENROUTER_API_KEY"
	}
	apiKey := os.Getenv(apiKeyEnvVar)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key found — set the %s environment variable", apiKeyEnvVar)
	}

	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 60 * time.Second},
	}, nil
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete sends a single-turn system+user prompt and returns the reply text.
func (c *Client) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return "", fmt.Errorf("error encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("error building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("error calling LLM API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("error reading LLM response: %w", err)
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("error parsing LLM response (status %d): %w", resp.StatusCode, err)
	}

	if parsed.Error != nil {
		return "", fmt.Errorf("LLM API error: %s", parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("LLM API returned an empty response")
	}

	return parsed.Choices[0].Message.Content, nil
}
