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

func TestRunCodingStoresFullToolTranscript(t *testing.T) {
	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatCalls++
		response := ollama.ChatResponse{Done: true}
		if chatCalls == 1 {
			response.Message = ollama.Message{
				Role:      "assistant",
				Thinking:  "I should inspect the file.",
				ToolCalls: []ollama.ToolCall{{Function: ollama.ToolFunctionCall{Name: "read_file", Arguments: map[string]any{"path": "source.txt"}}}},
			}
		} else {
			response.Message = ollama.Message{Role: "assistant", Content: "Done."}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	caseDir := t.TempDir()
	fixture := filepath.Join(caseDir, "fixture")
	if err := os.MkdirAll(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "source.txt"), []byte("fixture contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	caseJSON := `{"id":"coding-transcript","category":"coding","difficulty":"small","task":"Inspect the fixture.","visible_test_command":["true"],"verify_command":["true"]}`
	casePath := filepath.Join(caseDir, "case.json")
	if err := os.WriteFile(casePath, []byte(caseJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &Runner{
		Config:        Config{MaxAgentSteps: 4, MaxOutputTokens: 64, TestTimeoutSeconds: 2},
		Client:        ollama.New(server.URL, time.Second),
		digests:       map[string]string{"model:test": "digest"},
		ollamaVersion: "test",
	}
	result, err := runner.runCoding(context.Background(), "model:test", casePath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || len(result.Transcript) != 5 {
		t.Fatalf("passed=%v transcript=%#v", result.Passed, result.Transcript)
	}
	if result.Transcript[2].Thinking != "I should inspect the file." {
		t.Fatalf("assistant transcript = %#v", result.Transcript[2])
	}
	tool := result.Transcript[3]
	if tool.ToolName != "read_file" || tool.Content != "fixture contents" || tool.Arguments["path"] != "source.txt" {
		t.Fatalf("tool transcript = %#v", tool)
	}
}
