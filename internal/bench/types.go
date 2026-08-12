package bench

import "time"

type Config struct {
	OllamaURL     string   `json:"ollama_url"`
	ContextSize   int      `json:"context_size"`
	Temperature   float64  `json:"temperature"`
	MaxAgentSteps int      `json:"max_agent_steps"`
	Models        []string `json:"models"`
}

type Metrics struct {
	TotalDurationNS    int64   `json:"total_duration_ns"`
	LoadDurationNS     int64   `json:"load_duration_ns"`
	PromptTokens       int64   `json:"prompt_tokens"`
	PromptDurationNS   int64   `json:"prompt_duration_ns"`
	OutputTokens       int64   `json:"output_tokens"`
	OutputDurationNS   int64   `json:"output_duration_ns"`
	PromptTokensPerSec float64 `json:"prompt_tokens_per_sec"`
	OutputTokensPerSec float64 `json:"output_tokens_per_sec"`
	WallDurationMS     int64   `json:"wall_duration_ms"`
}

const BenchmarkVersion = "0.1.0"

type Result struct {
	Timestamp        time.Time `json:"timestamp"`
	RunID            string    `json:"run_id"`
	BenchmarkVersion string    `json:"benchmark_version"`
	Model            string    `json:"model"`
	ModelDigest      string    `json:"model_digest,omitempty"`
	CaseID           string    `json:"case_id"`
	Category         string    `json:"category"`
	Difficulty       string    `json:"difficulty,omitempty"`
	Passed           bool      `json:"passed"`
	Score            float64   `json:"score"`
	Details          string    `json:"details,omitempty"`
	Metrics          Metrics   `json:"metrics"`
	Before           any       `json:"before,omitempty"`
	After            any       `json:"after,omitempty"`
	ContextSize      int       `json:"context_size"`
	Temperature      float64   `json:"temperature"`
	AgentSteps       int       `json:"agent_steps,omitempty"`
	OllamaVersion    string    `json:"ollama_version,omitempty"`
}
