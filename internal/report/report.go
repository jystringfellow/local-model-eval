package report

import (
	"fmt"
	"sort"

	"github.com/example/local-model-eval/internal/bench"
)

type agg struct {
	n, pass, errors int
	formatN         int
	formatPass      int
	completed       int
	score           float64
	wall            []float64
	promptTPS       []float64
	outTPS          []float64
}

func Print(results []bench.Result) {
	by := map[string]map[string]*agg{}
	for _, r := range results {
		if by[r.Model] == nil {
			by[r.Model] = map[string]*agg{}
		}
		a := by[r.Model][r.Category]
		if a == nil {
			a = &agg{}
			by[r.Model][r.Category] = a
		}
		a.n++
		if r.Passed {
			a.pass++
		}
		if r.Error != "" {
			a.errors++
		}
		if r.FormatPassed != nil {
			a.formatN++
			if *r.FormatPassed {
				a.formatPass++
			}
		}
		a.score += r.Score
		if r.Error == "" {
			a.completed++
			a.wall = append(a.wall, float64(r.Metrics.WallDurationMS)/1000)
			if r.Metrics.PromptRateValid {
				a.promptTPS = append(a.promptTPS, r.Metrics.PromptTokensPerSec)
			}
			a.outTPS = append(a.outTPS, r.Metrics.OutputTokensPerSec)
		}
	}
	models := make([]string, 0, len(by))
	for m := range by {
		models = append(models, m)
	}
	sort.Strings(models)
	catsSet := map[string]bool{}
	for _, m := range models {
		for c := range by[m] {
			catsSet[c] = true
		}
	}
	cats := make([]string, 0, len(catsSet))
	for c := range catsSet {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	fmt.Printf("%-38s %-15s %7s %7s %5s %8s %9s %10s %10s\n", "MODEL", "CATEGORY", "PASS", "FORMAT", "ERROR", "SCORE", "WALL(s)", "PREFILL/s", "DECODE/s")
	for _, m := range models {
		for _, c := range cats {
			a := by[m][c]
			if a == nil {
				continue
			}
			n := float64(a.n)
			format := "-"
			if a.formatN > 0 {
				format = fmt.Sprintf("%d/%d", a.formatPass, a.formatN)
			}
			prefill := prefillDisplay(a)
			fmt.Printf("%-38s %-15s %3d/%-3d %7s %5d %8.3f %9.1f %10s %10.1f\n", m, c, a.pass, a.n, format, a.errors, a.score/n, median(a.wall), prefill, median(a.outTPS))
		}
	}
}

func prefillDisplay(a *agg) string {
	if a.completed == 0 || len(a.promptTPS) != a.completed {
		return "N/A"
	}
	return fmt.Sprintf("%.1f", median(a.promptTPS))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	values = append([]float64(nil), values...)
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}
