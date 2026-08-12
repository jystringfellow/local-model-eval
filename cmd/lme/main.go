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
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		modelsArg := fs.String("models", "", "comma-separated models (default: bench.json)")
		category := fs.String("category", "", "coding, classification, or basic")
		ctxSize := fs.Int("ctx", 0, "override context size")
		repeats := fs.Int("repeats", 1, "repeat every model/case N times")
		_ = fs.Parse(os.Args[2:])
		models := cfg.Models
		if *modelsArg != "" {
			models = splitCSV(*modelsArg)
		}
		if *ctxSize > 0 {
			cfg.ContextSize = *ctxSize
		}
		r := bench.NewRunner(root, cfg)
		for i := 0; i < *repeats; i++ {
			if *repeats > 1 {
				fmt.Printf("=== repeat %d/%d ===\n", i+1, *repeats)
			}
			_, err := r.Run(context.Background(), models, *category)
			if err != nil {
				fatal(err)
			}
		}
	case "report":
		fs := flag.NewFlagSet("report", flag.ExitOnError)
		all := fs.Bool("all", false, "aggregate all historical runs")
		runID := fs.String("run", "", "report a specific run id")
		_ = fs.Parse(os.Args[2:])
		results, err := bench.ReadResults(filepath.Join(root, "results", "results.jsonl"))
		if err != nil {
			fatal(err)
		}
		if !*all {
			target := *runID
			if target == "" {
				for _, rr := range results {
					if rr.RunID > target {
						target = rr.RunID
					}
				}
			}
			var filtered []bench.Result
			for _, rr := range results {
				if rr.RunID == target {
					filtered = append(filtered, rr)
				}
			}
			results = filtered
			if target != "" {
				fmt.Println("run:", target)
			}
		}
		report.Print(results)
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
  lme list
  lme run [--models a,b] [--category coding|classification|basic] [--ctx 65536] [--repeats 3]
  lme report [--run ID | --all]
  lme env`)
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
	fmt.Println("Ollama API: OK")
	fmt.Println("Ollama:", bench.OllamaVersion())
	fmt.Printf("Configured models: %d\nContext: %d\n", len(cfg.Models), cfg.ContextSize)
}
func goVersion() string {
	b, _ := exec.Command("go", "version").CombinedOutput()
	return strings.TrimSpace(string(b))
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "error:", err); os.Exit(1) }
