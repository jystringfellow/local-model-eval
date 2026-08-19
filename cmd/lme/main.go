package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/example/local-model-eval/internal/bench"
	"github.com/example/local-model-eval/internal/report"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	root := findRoot()
	cfg, err := bench.LoadConfig(filepath.Join(root, "bench.json"))
	if err != nil {
		fatal(err)
	}
	switch os.Args[1] {
	case "doctor":
		doctor(cfg)
	case "list":
		r := bench.NewRunner(root, cfg)
		cases, err := r.Cases("")
		if err != nil {
			fatal(err)
		}
		for _, c := range cases {
			rel, _ := filepath.Rel(root, c)
			fmt.Println(rel)
		}
	case "models":
		models, err := bench.InstalledModels()
		if err != nil {
			fatal(err)
		}
		for _, model := range models {
			fmt.Println(model)
		}
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		modelsArg := fs.String("models", "", "comma-separated models (default: all models from ollama list)")
		category := fs.String("category", "", "coding, classification, or basic")
		ctxSize := fs.Int("ctx", 0, "override context size")
		repeats := fs.Int("repeats", 1, "repeat every model/case N times")
		diagnose := fs.Bool("diagnose", true, "print an offline diagnosis after the run")
		_ = fs.Parse(os.Args[2:])
		var models []string
		if *modelsArg == "" {
			models, err = bench.InstalledModels()
			if err != nil {
				fatal(err)
			}
		} else {
			models = splitCSV(*modelsArg)
		}
		if len(models) == 0 {
			fatal(fmt.Errorf("no models selected"))
		}
		if *repeats <= 0 {
			fatal(fmt.Errorf("repeats must be positive"))
		}
		fmt.Printf("Models (%d): %s\n", len(models), strings.Join(models, ", "))
		if *ctxSize > 0 {
			cfg.ContextSize = *ctxSize
		}
		r := bench.NewRunner(root, cfg)
		fmt.Println("Run:", r.RunID)
		errorCount := 0
		var runResults []bench.Result
		for i := 0; i < *repeats; i++ {
			if *repeats > 1 {
				fmt.Printf("=== repeat %d/%d ===\n", i+1, *repeats)
			}
			results, err := r.Run(context.Background(), models, *category)
			if err != nil {
				fatal(err)
			}
			for _, result := range results {
				if result.Error != "" {
					errorCount++
				}
			}
			runResults = append(runResults, results...)
		}
		if *diagnose {
			fmt.Println()
			fmt.Print(report.Diagnose(runResults))
		}
		if errorCount > 0 {
			fatal(fmt.Errorf("run completed with %d infrastructure error(s); successful and failed case results were preserved", errorCount))
		}
	case "report":
		fs := flag.NewFlagSet("report", flag.ExitOnError)
		all := fs.Bool("all", false, "aggregate all historical runs")
		runID := fs.String("run", "", "report a specific run id")
		_ = fs.Parse(os.Args[2:])
		results, target := selectedResults(root, *runID, *all)
		if target != "" {
			fmt.Println("run:", target)
		}
		report.Print(results)
	case "diagnose":
		fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
		all := fs.Bool("all", false, "diagnose all historical runs for the current benchmark version")
		runID := fs.String("run", "", "diagnose a specific run id")
		_ = fs.Parse(os.Args[2:])
		results, target := selectedResults(root, *runID, *all)
		if target != "" {
			fmt.Println("run:", target)
		}
		fmt.Print(report.Diagnose(results))
	case "env":
		env := map[string]any{"ollama_version": bench.OllamaVersion(), "go_version": goVersion(), "config": cfg}
		b, _ := json.MarshalIndent(env, "", "  ")
		fmt.Println(string(b))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`local-model-eval (lme)

Commands:
  lme doctor
  lme list                                      # benchmark cases
  lme models                                    # locally installed models
  lme run [--models a,b] [--category coding|classification|basic] [--ctx 32768] [--repeats 3] [--diagnose=false]
  lme report [--run ID | --all]
  lme diagnose [--run ID | --all]
  lme env`)
}

func selectedResults(root, runID string, all bool) ([]bench.Result, string) {
	results, err := bench.ReadResults(filepath.Join(root, "results", "results.jsonl"))
	if err != nil {
		fatal(err)
	}
	if all {
		var filtered []bench.Result
		for _, result := range results {
			if result.BenchmarkVersion == bench.BenchmarkVersion {
				filtered = append(filtered, result)
			}
		}
		return filtered, ""
	}
	target := runID
	if target == "" {
		for _, result := range results {
			if result.RunID > target {
				target = result.RunID
			}
		}
	}
	var filtered []bench.Result
	for _, result := range results {
		if result.RunID == target {
			filtered = append(filtered, result)
		}
	}
	return filtered, target
}
func splitCSV(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
func findRoot() string {
	wd, _ := os.Getwd()
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "bench.json")); err == nil {
			return d
		}
		p := filepath.Dir(d)
		if p == d {
			break
		}
	}
	fatal(fmt.Errorf("bench.json not found; run from the project directory"))
	return ""
}
func doctor(cfg bench.Config) {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(strings.TrimRight(cfg.OllamaURL, "/") + "/api/tags")
	if err != nil {
		fatal(fmt.Errorf("ollama unreachable: %w", err))
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		fatal(fmt.Errorf("ollama returned %s", resp.Status))
	}
	models, err := bench.InstalledModels()
	if err != nil {
		fatal(err)
	}
	fmt.Println("Ollama API: OK")
	fmt.Println("Ollama:", bench.OllamaVersion())
	fmt.Printf("Installed models: %d\nContext: %d\n", len(models), cfg.ContextSize)
}
func goVersion() string {
	b, _ := exec.Command("go", "version").CombinedOutput()
	return strings.TrimSpace(string(b))
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }
