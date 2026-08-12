package report

import (
	"fmt"
	"sort"

	"github.com/example/local-model-eval/internal/bench"
)

type agg struct {
	n, pass                        int
	score, wall, promptTPS, outTPS float64
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
		a.score += r.Score
		a.wall += float64(r.Metrics.WallDurationMS) / 1000
		a.promptTPS += r.Metrics.PromptTokensPerSec
		a.outTPS += r.Metrics.OutputTokensPerSec
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
	fmt.Printf("%-38s %-15s %7s %8s %9s %10s %10s\n", "MODEL", "CATEGORY", "PASS", "SCORE", "WALL(s)", "PREFILL/s", "DECODE/s")
	for _, m := range models {
		for _, c := range cats {
			a := by[m][c]
			if a == nil {
				continue
			}
			n := float64(a.n)
			fmt.Printf("%-38s %-15s %3d/%-3d %8.3f %9.1f %10.1f %10.1f\n", m, c, a.pass, a.n, a.score/n, a.wall/n, a.promptTPS/n, a.outTPS/n)
		}
	}
}
