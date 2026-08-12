package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
	"github.com/example/local-model-eval/internal/telemetry"
)

type classificationCase struct {
	ID, Category, Description string
	Projects                  []map[string]any  `json:"projects"`
	Artifacts                 []map[string]any  `json:"artifacts"`
	Expected                  map[string]string `json:"expected"`
}

type assignmentResponse struct {
	Assignments []struct {
		ArtifactID string `json:"artifact_id"`
		ProjectID  string `json:"project_id"`
	} `json:"assignments"`
}

func (r *Runner) runClassification(ctx context.Context, model, path string) (Result, error) {
	var c classificationCase
	b, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return Result{}, err
	}
	input, _ := json.MarshalIndent(map[string]any{"projects": c.Projects, "artifacts": c.Artifacts}, "", "  ")
	prompt := fmt.Sprintf("%s\nAssign every artifact to exactly one project id, or 'unrelated'. Use semantic evidence across people, topics, codes, and wording; do not force weak matches. Return only the requested structured data.\n\n%s", c.Description, string(input))
	schema := map[string]any{"type": "object", "properties": map[string]any{"assignments": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"artifact_id": map[string]any{"type": "string"}, "project_id": map[string]any{"type": "string"}}, "required": []string{"artifact_id", "project_id"}}}}, "required": []string{"assignments"}}
	res := r.baseResult(ctx, model, c.ID, c.Category, "")
	before := telemetry.Capture()
	start := time.Now()
	resp, err := r.Client.Chat(ctx, ollama.ChatRequest{Model: model, Messages: []ollama.Message{{Role: "user", Content: prompt}}, Format: schema, Stream: false, Options: options(r.Config)})
	wall := time.Since(start)
	after := telemetry.Capture()
	if err != nil {
		return res, err
	}
	var got assignmentResponse
	parseErr := json.Unmarshal([]byte(resp.Message.Content), &got)
	correct := 0
	preds := map[string]string{}
	for _, a := range got.Assignments {
		preds[a.ArtifactID] = a.ProjectID
	}
	for id, want := range c.Expected {
		if preds[id] == want {
			correct++
		}
	}
	res.Score = float64(correct) / float64(len(c.Expected))
	res.Passed = parseErr == nil && correct == len(c.Expected)
	keys := make([]string, 0, len(c.Expected))
	for k := range c.Expected {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	details := ""
	for _, k := range keys {
		details += fmt.Sprintf("%s=%s(want:%s) ", k, preds[k], c.Expected[k])
	}
	if parseErr != nil {
		details += " parse_error=" + parseErr.Error()
	}
	res.Details = details
	res.Metrics = metricsFrom(resp, wall)
	res.Before = before
	res.After = after
	return res, nil
}
