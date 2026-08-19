package bench

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
)

func TestBasicCaseExpectedExactJSONTag(t *testing.T) {
	var c basicCase
	if err := json.Unmarshal([]byte(`{"id":"x","category":"basic","prompt":"p","expected_exact":"yes"}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.ExpectedExact != "yes" {
		t.Fatalf("ExpectedExact = %q, want yes", c.ExpectedExact)
	}
}

func TestOptionsBoundOutput(t *testing.T) {
	got := options(Config{ContextSize: 32768, Temperature: 0, MaxOutputTokens: 4096})
	if got["num_ctx"] != 32768 || got["num_predict"] != 4096 {
		t.Fatalf("options() = %#v", got)
	}
}

func TestScoreClassificationRequiresExactlyOneAssignment(t *testing.T) {
	c := classificationCase{
		Projects:  []map[string]any{{"id": "atlas"}},
		Artifacts: []map[string]any{{"id": "a"}},
		Expected:  map[string]string{"a": "atlas"},
	}
	response := `{"assignments":[{"artifact_id":"a","project_id":"atlas"},{"artifact_id":"a","project_id":"atlas"}]}`
	outcome := scoreClassification(c, response)
	if outcome.SemanticScore != 1 || !outcome.SemanticPassed || outcome.FormatPassed || !strings.Contains(outcome.Details, "duplicate artifact") {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestScoreClassificationPassesValidResponse(t *testing.T) {
	c := classificationCase{
		Projects:  []map[string]any{{"id": "atlas"}},
		Artifacts: []map[string]any{{"id": "a"}, {"id": "b"}},
		Expected:  map[string]string{"a": "atlas", "b": "unrelated"},
	}
	response := `{"assignments":[{"artifact_id":"a","project_id":"atlas"},{"artifact_id":"b","project_id":"unrelated"}]}`
	outcome := scoreClassification(c, response)
	if outcome.SemanticScore != 1 || !outcome.SemanticPassed || !outcome.FormatPassed {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestScoreClassificationRecoversFencedAliases(t *testing.T) {
	c := classificationCase{
		Projects:  []map[string]any{{"id": "atlas"}},
		Artifacts: []map[string]any{{"id": "a"}, {"id": "b"}},
		Expected:  map[string]string{"a": "atlas", "b": "unrelated"},
	}
	response := "json\n{```json\n{\"assignments\":[{\"id\":\"a\",\"projectId\":\"atlas\"},{\"id\":\"b\",\"project_id\":\"unrelated\"}]}\n```"
	outcome := scoreClassification(c, response)
	if outcome.SemanticScore != 1 || !outcome.SemanticPassed || outcome.FormatPassed {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestMetricsRejectImplausiblePromptDuration(t *testing.T) {
	metrics := metricsFrom(ollama.ChatResponse{PromptEvalCount: 2000, PromptEvalDuration: 7_000}, time.Second)
	if metrics.PromptRateValid || metrics.PromptTokensPerSec != 0 {
		t.Fatalf("metrics = %#v", metrics)
	}
	metrics = metricsFrom(ollama.ChatResponse{PromptEvalCount: 2000, PromptEvalDuration: int64(time.Second)}, time.Second)
	if !metrics.PromptRateValid || metrics.PromptTokensPerSec != 2000 {
		t.Fatalf("metrics = %#v", metrics)
	}
}
