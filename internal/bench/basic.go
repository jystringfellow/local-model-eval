package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
	"github.com/example/local-model-eval/internal/telemetry"
)

type basicCase struct {
	ID            string `json:"id"`
	Category      string `json:"category"`
	Prompt        string `json:"prompt"`
	ExpectedExact string `json:"expected_exact"`
}

func (r *Runner) runBasic(ctx context.Context, model, path string) (Result, error) {
	var c basicCase
	b, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Result{}, err
	}
	if c.ID == "" || c.Category != "basic" || c.Prompt == "" || c.ExpectedExact == "" {
		return Result{}, fmt.Errorf("%s: invalid basic case", path)
	}
	res := r.baseResult(ctx, model, c.ID, c.Category, "")
	before := telemetry.Capture()
	start := time.Now()
	resp, err := r.Client.Chat(ctx, ollama.ChatRequest{Model: model, Messages: []ollama.Message{{Role: "user", Content: c.Prompt}}, Stream: false, Options: options(r.Config)})
	wall := time.Since(start)
	after := telemetry.Capture()
	res.Metrics = metricsFrom(resp, wall)
	res.Before = before
	res.After = after
	if err != nil {
		return res, err
	}
	got := strings.TrimSpace(resp.Message.Content)
	res.Passed = got == c.ExpectedExact
	if res.Passed {
		res.Score = 1
	}
	res.Details = "response=" + got
	return res, nil
}
