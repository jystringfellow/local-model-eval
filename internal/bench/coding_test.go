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
	if !result.Passed || len(result.Transcript) != 9 {
		t.Fatalf("passed=%v transcript=%#v", result.Passed, result.Transcript)
	}
	if result.Transcript[2].Thinking != "I should inspect the file." {
		t.Fatalf("assistant transcript = %#v", result.Transcript[2])
	}
	tool := result.Transcript[3]
	if tool.ToolName != "read_file" || tool.Content != "fixture contents" || tool.Arguments["path"] != "source.txt" {
		t.Fatalf("tool transcript = %#v", tool)
	}
	// The model never wrote a file, so the no-write nudge must fire exactly
	// codingNudgeLimit times and each nudge must be recorded in the transcript.
	nudges := 0
	for _, e := range result.Transcript {
		if e.Role == "user" && e.Content == codingNudge() {
			nudges++
		}
	}
	if nudges != codingNudgeLimit {
		t.Fatalf("nudges = %d, want %d; transcript=%#v", nudges, codingNudgeLimit, result.Transcript)
	}
}

func TestRunCodingNudgesWhenNoWrite(t *testing.T) {
	// The complement of the transcript test: a model that writes a file must NOT
	// be nudged. A write is a genuine attempt, so a later text-only turn is a
	// real completion, not a give-up.
	chatCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatCalls++
		response := ollama.ChatResponse{Done: true}
		switch chatCalls {
		case 1:
			response.Message = ollama.Message{Role: "assistant", ToolCalls: []ollama.ToolCall{{Function: ollama.ToolFunctionCall{Name: "write_file", Arguments: map[string]any{"path": "solution.txt", "content": "package main"}}}}}
		default:
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
	caseJSON := `{"id":"coding-nudge","category":"coding","difficulty":"small","task":"Write a solution.","visible_test_command":["true"],"verify_command":["true"]}`
	casePath := filepath.Join(caseDir, "case.json")
	if err := os.WriteFile(casePath, []byte(caseJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &Runner{
		Config:        Config{MaxAgentSteps: 8, MaxOutputTokens: 64, TestTimeoutSeconds: 2},
		Client:        ollama.New(server.URL, time.Second),
		digests:       map[string]string{"model:test": "digest"},
		ollamaVersion: "test",
	}
	result, err := runner.runCoding(context.Background(), "model:test", casePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range result.Transcript {
		if e.Role == "user" && e.Content == codingNudge() {
			t.Fatalf("unexpected nudge after a write in transcript=%#v", result.Transcript)
		}
	}
	// The write happens at step 1, the "Done." turn at step 2 breaks the loop.
	if chatCalls != 2 {
		t.Fatalf("chatCalls = %d, want 2", chatCalls)
	}
}

func TestParseSubtests(t *testing.T) {
	// A verbose subtest run counts only subtests (names containing "/"), not the
	// parent, and a FAIL subtest counts toward total but not toward passed.
	verbose := `=== RUN   TestHiddenBucketing
--- PASS: TestHiddenBucketing/a (0.00s)
--- PASS: TestHiddenBucketing/b (0.00s)
--- FAIL: TestHiddenBucketing/c (0.00s)
--- PASS: TestHiddenBucketing/d (0.00s)
--- FAIL: TestHiddenBucketing (0.01s)`
	if passed, total, ok := parseSubtests(verbose); !ok || passed != 3 || total != 4 {
		t.Fatalf("parseSubtests(verbose) = %d/%d ok=%v, want 3/4 ok=true", passed, total, ok)
	}
	// No subtest output (empty output or a build failure) falls back to binary.
	if _, _, ok := parseSubtests(""); ok {
		t.Fatalf("parseSubtests(empty) ok=true, want false")
	}
	buildFail := "# github.com/example/bucket\n./bucket.go:10:2: undefined: strings\nFAIL\tgithub.com/example/bucket\t0.003s"
	if _, _, ok := parseSubtests(buildFail); ok {
		t.Fatalf("parseSubtests(buildFail) ok=true, want false")
	}
	// A parent-only test with no subtests is not partial-credit-able.
	if _, _, ok := parseSubtests("--- PASS: TestExplicitCode (0.00s)"); ok {
		t.Fatalf("parseSubtests(parentOnly) ok=true, want false")
	}
}
