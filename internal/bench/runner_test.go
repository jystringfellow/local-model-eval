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

func TestRunRecordsChatErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && req.URL.Path == "/api/ps" {
			_, _ = w.Write([]byte(`{"models":[]}`))
			return
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, unloading := body["keep_alive"]; unloading {
			_ = json.NewEncoder(w).Encode(ollama.ChatResponse{Done: true})
			return
		}
		http.Error(w, `{"error":"model crashed"}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	root := t.TempDir()
	caseDir := filepath.Join(root, "_cases", "basic")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	caseJSON := `{"id":"basic-error","category":"basic","prompt":"yes?","expected_exact":"yes"}`
	if err := os.WriteFile(filepath.Join(caseDir, "case.json"), []byte(caseJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &Runner{
		Root:          root,
		RunID:         "test-run",
		Config:        Config{ContextSize: 1024, MaxOutputTokens: 64, TestTimeoutSeconds: 1},
		Client:        ollama.New(server.URL, time.Second),
		digests:       map[string]string{"model:test": "digest"},
		ollamaVersion: "test",
	}
	results, err := runner.Run(context.Background(), []string{"model:test"}, "basic")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("results = %#v, want one recorded error", results)
	}
	persisted, err := ReadResults(filepath.Join(root, "results", "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || persisted[0].Error == "" {
		t.Fatalf("persisted results = %#v", persisted)
	}
}
