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
)

type Runner struct {
	Root          string
	RunID         string
	Config        Config
	Client        *ollama.Client
	digests       map[string]string
	ollamaVersion string
}

func NewRunner(root string, cfg Config) *Runner {
	return &Runner{
		Root:          root,
		RunID:         time.Now().Format("20060102-150405.000"),
		Config:        cfg,
		Client:        ollama.New(cfg.OllamaURL, time.Duration(cfg.ChatTimeoutSeconds)*time.Second),
		digests:       make(map[string]string),
		ollamaVersion: OllamaVersion(),
	}
}

func (r *Runner) Cases(category string) ([]string, error) {
	if category != "" && category != "basic" && category != "classification" && category != "coding" {
		return nil, fmt.Errorf("unknown category %q", category)
	}
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
			if err := json.Unmarshal(b, &h); err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			if h.Category == "" {
				return fmt.Errorf("%s: missing category", path)
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

type caseHeader struct {
	ID       string `json:"id"`
	Category string `json:"category"`
}

type caseSpec struct {
	path   string
	header caseHeader
}

func (r *Runner) Run(ctx context.Context, models []string, category string) ([]Result, error) {
	if len(models) == 0 {
		return nil, fmt.Errorf("no models selected")
	}
	cases, err := r.Cases(category)
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("no benchmark cases found for category %q", category)
	}
	if err := r.unloadSelectedRunningModels(ctx, models); err != nil {
		return nil, fmt.Errorf("prepare model memory: %w", err)
	}
	specs := make([]caseSpec, 0, len(cases))
	for _, path := range cases {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var h caseHeader
		if err := json.Unmarshal(b, &h); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if h.ID == "" {
			return nil, fmt.Errorf("%s: missing id", path)
		}
		specs = append(specs, caseSpec{path: path, header: h})
	}
	var results []Result
	for _, model := range models {
		for _, spec := range specs {
			fmt.Printf("[%s] %s ... ", model, spec.header.ID)
			var res Result
			switch spec.header.Category {
			case "coding":
				res, err = r.runCoding(ctx, model, spec.path)
			case "classification":
				res, err = r.runClassification(ctx, model, spec.path)
			case "basic":
				res, err = r.runBasic(ctx, model, spec.path)
			default:
				return results, fmt.Errorf("%s: unknown category %q", spec.path, spec.header.Category)
			}
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
				if res.CaseID == "" {
					return results, err
				}
				res.Error = err.Error()
				if res.Details == "" {
					res.Details = "infrastructure_error: " + err.Error()
				}
			} else if res.Error != "" {
				fmt.Printf("ERROR: %s", res.Error)
			} else if res.Passed {
				fmt.Printf("PASS")
			} else {
				fmt.Printf("FAIL")
			}
			if err == nil {
				if res.FormatPassed != nil {
					fmt.Printf(" semantic=%.2f format=%t", res.Score, *res.FormatPassed)
				} else {
					fmt.Printf(" score=%.2f", res.Score)
				}
				fmt.Printf(" wall=%.1fs out=%.1f tok/s\n", float64(res.Metrics.WallDurationMS)/1000, res.Metrics.OutputTokensPerSec)
			}
			if err := r.appendResult(res); err != nil {
				return results, err
			}
			results = append(results, res)
			if res.Error != "" {
				if err := r.unloadModel(model); err != nil {
					return results, fmt.Errorf("reset %s after error: %w", model, err)
				}
			}
		}
		if err := r.unloadModel(model); err != nil {
			return results, fmt.Errorf("unload %s: %w", model, err)
		}
	}
	return results, nil
}

func (r *Runner) unloadSelectedRunningModels(ctx context.Context, selected []string) error {
	running, err := r.Client.RunningModels(ctx)
	if err != nil {
		return err
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, model := range selected {
		selectedSet[model] = true
	}
	for _, model := range running {
		if !selectedSet[model] {
			fmt.Printf("[%s] WARNING: model is already loaded but not selected; it may affect memory measurements\n", model)
			continue
		}
		if err := r.unloadModel(model); err != nil {
			return fmt.Errorf("unload %s: %w", model, err)
		}
	}
	return nil
}

func (r *Runner) unloadModel(model string) error {
	unloadCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return r.Client.Unload(unloadCtx, model)
}

const minimumReliablePromptDuration = time.Millisecond

func metricsFrom(resp ollama.ChatResponse, wall time.Duration) Metrics {
	m := Metrics{TotalDurationNS: resp.TotalDuration, LoadDurationNS: resp.LoadDuration, PromptTokens: resp.PromptEvalCount, PromptDurationNS: resp.PromptEvalDuration, OutputTokens: resp.EvalCount, OutputDurationNS: resp.EvalDuration, WallDurationMS: wall.Milliseconds()}
	if m.PromptTokens > 0 && m.PromptDurationNS >= minimumReliablePromptDuration.Nanoseconds() {
		m.PromptTokensPerSec = float64(m.PromptTokens) / (float64(m.PromptDurationNS) / 1e9)
		m.PromptRateValid = true
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
	a.PromptTokensPerSec = 0
	a.PromptRateValid = false
	if a.PromptTokens > 0 && a.PromptDurationNS >= minimumReliablePromptDuration.Nanoseconds() {
		a.PromptTokensPerSec = float64(a.PromptTokens) / (float64(a.PromptDurationNS) / 1e9)
		a.PromptRateValid = true
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
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Result
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 32*1024*1024)
	line := 0
	for s.Scan() {
		line++
		var r Result
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		out = append(out, r)
	}
	return out, s.Err()
}

func (r *Runner) baseResult(ctx context.Context, model, caseID, category, difficulty string) Result {
	digest, ok := r.digests[model]
	if !ok {
		digest = r.Client.ModelDigest(ctx, model)
		r.digests[model] = digest
	}
	return Result{Timestamp: time.Now(), RunID: r.RunID, BenchmarkVersion: BenchmarkVersion, Model: model, ModelDigest: digest, CaseID: caseID, Category: category, Difficulty: difficulty, ContextSize: r.Config.ContextSize, Temperature: r.Config.Temperature, OllamaVersion: r.ollamaVersion}
}

func options(cfg Config) map[string]any {
	return map[string]any{"temperature": cfg.Temperature, "num_ctx": cfg.ContextSize, "num_predict": cfg.MaxOutputTokens}
}
