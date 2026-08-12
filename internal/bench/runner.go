package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
	"github.com/example/local-model-eval/internal/telemetry"
)

type Runner struct {
	Root   string
	RunID  string
	Config Config
	Client *ollama.Client
}

func NewRunner(root string, cfg Config) *Runner {
	return &Runner{Root: root, RunID: time.Now().Format("20060102-150405"), Config: cfg, Client: ollama.New(cfg.OllamaURL)}
}

func (r *Runner) Cases(category string) ([]string, error) {
	var paths []string
	base := filepath.Join(r.Root, "_cases")
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "case.json" || (strings.HasSuffix(path, ".json") && !strings.Contains(path, string(filepath.Separator)+"coding"+string(filepath.Separator))) {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			var h struct {
				Category string `json:"category"`
			}
			if json.Unmarshal(b, &h) != nil {
				return nil
			}
			if category == "" || h.Category == category {
				paths = append(paths, path)
			}
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func (r *Runner) Run(ctx context.Context, models []string, category string) ([]Result, error) {
	cases, err := r.Cases(category)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, model := range models {
		for _, path := range cases {
			b, _ := os.ReadFile(path)
			var h struct {
				Category string `json:"category"`
			}
			_ = json.Unmarshal(b, &h)
			fmt.Printf("[%s] %s ... ", model, filepath.Base(filepath.Dir(path)))
			var res Result
			switch h.Category {
			case "coding":
				res, err = r.runCoding(ctx, model, path)
			case "classification":
				res, err = r.runClassification(ctx, model, path)
			case "basic":
				res, err = r.runBasic(ctx, model, path)
			default:
				err = fmt.Errorf("unknown category %q", h.Category)
			}
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
				continue
			}
			if res.Passed {
				fmt.Printf("PASS")
			} else {
				fmt.Printf("FAIL")
			}
			fmt.Printf(" score=%.2f wall=%.1fs out=%.1f tok/s\n", res.Score, float64(res.Metrics.WallDurationMS)/1000, res.Metrics.OutputTokensPerSec)
			if err := r.appendResult(res); err != nil {
				return results, err
			}
			results = append(results, res)
		}
	}
	return results, nil
}

func metricsFrom(resp ollama.ChatResponse, wall time.Duration) Metrics {
	m := Metrics{TotalDurationNS: resp.TotalDuration, LoadDurationNS: resp.LoadDuration, PromptTokens: resp.PromptEvalCount, PromptDurationNS: resp.PromptEvalDuration, OutputTokens: resp.EvalCount, OutputDurationNS: resp.EvalDuration, WallDurationMS: wall.Milliseconds()}
	if m.PromptDurationNS > 0 {
		m.PromptTokensPerSec = float64(m.PromptTokens) / (float64(m.PromptDurationNS) / 1e9)
	}
	if m.OutputDurationNS > 0 {
		m.OutputTokensPerSec = float64(m.OutputTokens) / (float64(m.OutputDurationNS) / 1e9)
	}
	return m
}

func addMetrics(a *Metrics, resp ollama.ChatResponse, wall time.Duration) {
	a.TotalDurationNS += resp.TotalDuration
	a.LoadDurationNS += resp.LoadDuration
	a.PromptTokens += resp.PromptEvalCount
	a.PromptDurationNS += resp.PromptEvalDuration
	a.OutputTokens += resp.EvalCount
	a.OutputDurationNS += resp.EvalDuration
	a.WallDurationMS += wall.Milliseconds()
	if a.PromptDurationNS > 0 {
		a.PromptTokensPerSec = float64(a.PromptTokens) / (float64(a.PromptDurationNS) / 1e9)
	}
	if a.OutputDurationNS > 0 {
		a.OutputTokensPerSec = float64(a.OutputTokens) / (float64(a.OutputDurationNS) / 1e9)
	}
}

func (r *Runner) appendResult(res Result) error {
	path := filepath.Join(r.Root, "results", "results.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(res)
}

func ReadResults(path string) ([]Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Result
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 4*1024*1024)
	for s.Scan() {
		var r Result
		if json.Unmarshal(s.Bytes(), &r) == nil {
			out = append(out, r)
		}
	}
	return out, s.Err()
}

func (r *Runner) baseResult(ctx context.Context, model, caseID, category, difficulty string) Result {
	return Result{Timestamp: time.Now(), RunID: r.RunID, BenchmarkVersion: BenchmarkVersion, Model: model, ModelDigest: r.Client.ModelDigest(ctx, model), CaseID: caseID, Category: category, Difficulty: difficulty, ContextSize: r.Config.ContextSize, Temperature: r.Config.Temperature, OllamaVersion: OllamaVersion()}
}

func options(cfg Config) map[string]any {
	return map[string]any{"temperature": cfg.Temperature, "num_ctx": cfg.ContextSize}
}

func snapshots() (any, any) { return telemetry.Capture(), nil }
