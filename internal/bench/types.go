package bench

import (
	"time"

	"github.com/example/local-model-eval/internal/ollama"
)

type Config struct {
	OllamaURL          string  `json:"ollama_url"`
	ContextSize        int     `json:"context_size"`
	Temperature        float64 `json:"temperature"`
	MaxAgentSteps      int     `json:"max_agent_steps"`
	MaxOutputTokens    int     `json:"max_output_tokens"`
	ChatTimeoutSeconds int     `json:"chat_timeout_seconds"`
	TestTimeoutSeconds int     `json:"test_timeout_seconds"`
}

type Metrics struct {
	TotalDurationNS    int64   `json:"total_duration_ns"`
	LoadDurationNS     int64   `json:"load_duration_ns"`
	PromptTokens       int64   `json:"prompt_tokens"`
	PromptDurationNS   int64   `json:"prompt_duration_ns"`
	OutputTokens       int64   `json:"output_tokens"`
	OutputDurationNS   int64   `json:"output_duration_ns"`
	PromptTokensPerSec float64 `json:"prompt_tokens_per_sec"`
	PromptRateValid    bool    `json:"prompt_rate_valid"`
	OutputTokensPerSec float64 `json:"output_tokens_per_sec"`
	WallDurationMS     int64   `json:"wall_duration_ms"`
}

const BenchmarkVersion = "0.3.0"

type TranscriptEntry struct {
	Step      int               `json:"step"`
	Role      string            `json:"role"`
	Content   string            `json:"content,omitempty"`
	Thinking  string            `json:"thinking,omitempty"`
	ToolCalls []ollama.ToolCall `json:"tool_calls,omitempty"`
	ToolName  string            `json:"tool_name,omitempty"`
	Arguments map[string]any    `json:"arguments,omitempty"`
}

type Result struct {
	Timestamp        time.Time         `json:"timestamp"`
	RunID            string            `json:"run_id"`
	BenchmarkVersion string            `json:"benchmark_version"`
	Model            string            `json:"model"`
	ModelDigest      string            `json:"model_digest,omitempty"`
	CaseID           string            `json:"case_id"`
	Category         string            `json:"category"`
	Difficulty       string            `json:"difficulty,omitempty"`
	Passed           bool              `json:"passed"`
	Score            float64           `json:"score"`
	FormatPassed     *bool             `json:"format_passed,omitempty"`
	Error            string            `json:"error,omitempty"`
	Details          string            `json:"details,omitempty"`
	RawResponse      string            `json:"raw_response,omitempty"`
	Transcript       []TranscriptEntry `json:"transcript,omitempty"`
	Metrics          Metrics           `json:"metrics"`
	Before           any               `json:"before,omitempty"`
	After            any               `json:"after,omitempty"`
	ContextSize      int               `json:"context_size"`
	Temperature      float64           `json:"temperature"`
	AgentSteps       int               `json:"agent_steps,omitempty"`
	OllamaVersion    string            `json:"ollama_version,omitempty"`
}
