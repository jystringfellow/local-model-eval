package bench

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
	"github.com/example/local-model-eval/internal/telemetry"
)

type basicCase struct{ ID, Category, Prompt, ExpectedExact string }

func (r *Runner) runBasic(ctx context.Context, model, path string) (Result, error) {
	var c basicCase
	b, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Result{}, err
	}
	res := r.baseResult(ctx, model, c.ID, c.Category, "")
	before := telemetry.Capture()
	start := time.Now()
	resp, err := r.Client.Chat(ctx, ollama.ChatRequest{Model: model, Messages: []ollama.Message{{Role: "user", Content: c.Prompt}}, Stream: false, Options: options(r.Config)})
	wall := time.Since(start)
	after := telemetry.Capture()
	if err != nil {
		return res, err
	}
	got := strings.TrimSpace(resp.Message.Content)
	res.Passed = got == c.ExpectedExact
	if res.Passed {
		res.Score = 1
	}
	res.Details = "response=" + got
	res.Metrics = metricsFrom(resp, wall)
	res.Before = before
	res.After = after
	return res, nil
}
