package bench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseOllamaList(t *testing.T) {
	output := `NAME                             ID              SIZE      MODIFIED
north-mini-code-1.0:mlx-mxfp8    82b3f0ab2d1c    31 GB     15 seconds ago
gemma4:12b-mlx                   117d0d84cf2a    7.7 GB    5 minutes ago
`
	want := []string{"north-mini-code-1.0:mlx-mxfp8", "gemma4:12b-mlx"}
	if got := parseOllamaList(output); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseOllamaList() = %v, want %v", got, want)
	}
}

func TestLoadConfigRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench.json")
	data := `{"ollama_url":"http://localhost:11434","context_size":32768,"temperature":0,"max_agent_steps":16,"max_output_tokens":4096,"chat_timeout_seconds":600,"test_timeout_seconds":120,"models":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig() accepted an unknown field")
	}
}

func TestRunCommandTimesOut(t *testing.T) {
	start := time.Now()
	_, err := runCommand(context.Background(), t.TempDir(), []string{"sh", "-c", "exec sleep 2"}, 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runCommand() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("runCommand() took %s after timeout", elapsed)
	}
}

func TestParseOllamaListEmpty(t *testing.T) {
	if got := parseOllamaList("NAME ID SIZE MODIFIED\n"); len(got) != 0 {
		t.Fatalf("parseOllamaList() = %v, want no models", got)
	}
}
