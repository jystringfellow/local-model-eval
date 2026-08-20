package report

import (
	"strings"
	"testing"

	"github.com/example/local-model-eval/internal/bench"
	"github.com/example/local-model-eval/internal/ollama"
)

func TestMedian(t *testing.T) {
	for _, tc := range []struct {
		values []float64
		want   float64
	}{
		{nil, 0},
		{[]float64{3}, 3},
		{[]float64{9, 1, 5}, 5},
		{[]float64{8, 2, 4, 6}, 5},
	} {
		if got := median(tc.values); got != tc.want {
			t.Fatalf("median(%v) = %v, want %v", tc.values, got, tc.want)
		}
	}
}

func TestPrefillDisplayRequiresEveryMeasurement(t *testing.T) {
	if got := prefillDisplay(&agg{completed: 2, promptTPS: []float64{100}}); got != "N/A" {
		t.Fatalf("prefillDisplay() = %q, want N/A", got)
	}
	if got := prefillDisplay(&agg{completed: 2, promptTPS: []float64{100, 200}}); got != "150.0" {
		t.Fatalf("prefillDisplay() = %q, want 150.0", got)
	}
}

func TestDiagnoseExplainsRecommendationsAndSignals(t *testing.T) {
	formatPass, formatFail := true, false
	results := []bench.Result{
		result("fast", "basic", "easy", true, 1, 10, 120),
		result("fast", "classification", "negative", false, .75, 20, 110, withFormat(&formatPass), withDetails("n3=atlas(want:unrelated)")),
		result("fast", "coding", "code", false, 0, 30, 100, withTranscript(writeCall()), withDetails("verifier:\nFAIL len 2")),
		result("quality", "basic", "easy", true, 1, 20, 60),
		result("quality", "classification", "negative", false, .75, 30, 55, withFormat(&formatFail), withDetails("n3=atlas(want:unrelated) format_error=bad")),
		result("quality", "coding", "code", true, 1, 60, 50),
	}

	got := Diagnose(results)
	for _, want := range []string{
		"6 samples, 2 models, 3 cases, 0 infrastructure errors",
		"rankings are directional",
		"Quality: quality",
		"Throughput: fast",
		"Structured output: fast (1/1 exact-format responses)",
		"No discrimination (all passed): easy",
		"Consensus semantic miss: negative/n3 predicted atlas, expected unrelated (2/2 samples)",
		"incomplete output or missing records",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Diagnose() missing %q:\n%s", want, got)
		}
	}
}

func TestDiagnoseFindsUnstableRepeatedOutcomeAndNoWrite(t *testing.T) {
	results := []bench.Result{
		result("model", "coding", "code", true, 1, 10, 10),
		result("model", "coding", "code", false, 0, 10, 10, withDetails("verifier: panic: TODO")),
		result("model", "coding", "code", false, 0, 10, 10, withDetails("verifier: panic: TODO")),
	}
	got := Diagnose(results)
	for _, want := range []string{
		"Evidence: 3 sample(s) per model/case",
		"Unstable outcomes across repeats (1): model / code",
		"2/2: no implementation attempted",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Diagnose() missing %q:\n%s", want, got)
		}
	}
}

type resultOption func(*bench.Result)

func result(model, category, caseID string, passed bool, score, wallSeconds, decodeTPS float64, options ...resultOption) bench.Result {
	r := bench.Result{
		RunID:    "run-1",
		Model:    model,
		Category: category,
		CaseID:   caseID,
		Passed:   passed,
		Score:    score,
		Metrics:  bench.Metrics{WallDurationMS: int64(wallSeconds * 1000), OutputTokensPerSec: decodeTPS},
	}
	for _, option := range options {
		option(&r)
	}
	return r
}

func withFormat(value *bool) resultOption {
	return func(r *bench.Result) { r.FormatPassed = value }
}

func withDetails(value string) resultOption {
	return func(r *bench.Result) { r.Details = value }
}

func withTranscript(entries ...bench.TranscriptEntry) resultOption {
	return func(r *bench.Result) { r.Transcript = entries }
}

func writeCall() bench.TranscriptEntry {
	return bench.TranscriptEntry{Role: "assistant", ToolCalls: []ollama.ToolCall{{Function: ollama.ToolFunctionCall{Name: "write_file"}}}}
}
