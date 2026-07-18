package summary

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"devlog/internal/domain"
)

func TestDeterministicAndOpenAICompatible(t *testing.T) {
	activities := []domain.Activity{{Description: "Implemented event sync", Confidence: domain.ConfidenceHigh}}
	plain, err := (Deterministic{}).Generate(context.Background(), "2026-07-12", activities)
	if err != nil || plain != "- Implemented event sync" {
		t.Fatalf("plain=%q err=%v", plain, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing auth")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Polished summary"}}]}`)
	}))
	defer server.Close()
	generated, err := (OpenAICompatible{BaseURL: server.URL, APIKey: "secret", Model: "test"}).Generate(context.Background(), "2026-07-12", activities)
	if err != nil || strings.TrimSpace(generated) != "Polished summary" {
		t.Fatalf("generated=%q err=%v", generated, err)
	}
}
