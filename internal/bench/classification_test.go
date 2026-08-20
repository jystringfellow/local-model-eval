package bench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
)

func TestRunClassificationStoresRawResponseAndSplitScores(t *testing.T) {
	raw := "```json\n{\"assignments\":[{\"id\":\"a\",\"projectId\":\"atlas\"}]}\n```"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ollama.ChatResponse{Done: true, Message: ollama.Message{Role: "assistant", Content: raw}})
	}))
	defer server.Close()

	caseJSON := `{"id":"classification-split","category":"classification","description":"Classify.","projects":[{"id":"atlas"}],"artifacts":[{"id":"a"}],"expected":{"a":"atlas"}}`
	casePath := filepath.Join(t.TempDir(), "case.json")
	if err := os.WriteFile(casePath, []byte(caseJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{
		Config:        Config{ContextSize: 1024, MaxOutputTokens: 64},
		Client:        ollama.New(server.URL, time.Second),
		digests:       map[string]string{"model:test": "digest"},
		ollamaVersion: "test",
	}
	result, err := runner.runClassification(context.Background(), "model:test", casePath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.Score != 1 || result.FormatPassed == nil || *result.FormatPassed || result.RawResponse != raw {
		t.Fatalf("result = %#v", result)
	}
}
