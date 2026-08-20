package report

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/example/local-model-eval/internal/bench"
)

const balancedQualityFraction = 0.85

type modelDiagnosis struct {
	name                string
	n                   int
	completed           int
	passes              int
	errors              int
	score               float64
	wallSeconds         float64
	decodeTPS           []float64
	caseIDs             map[string]bool
	classificationN     int
	classificationScore float64
	formatN             int
	formatPass          int
	codingN             int
	codingPass          int
}

func (m *modelDiagnosis) quality() float64 {
	if m.n == 0 {
		return 0
	}
	return m.score / float64(m.n)
}

func (m *modelDiagnosis) classificationQuality() float64 {
	if m.classificationN == 0 {
		return 0
	}
	return m.classificationScore / float64(m.classificationN)
}

func (m *modelDiagnosis) suiteWallSeconds() float64 {
	if m.completed == 0 {
		return 0
	}
	return m.wallSeconds * float64(len(m.caseIDs)) / float64(m.completed)
}

// Diagnose returns a deterministic, offline interpretation of benchmark results.
// The rules are deliberately simple and printed with the output so a recommendation
// can be audited without trusting another model or an opaque composite score.
func Diagnose(results []bench.Result) string {
	if len(results) == 0 {
		return "No results to diagnose.\n"
	}

	models := aggregateModels(results)
	runIDs, cases := uniqueRunAndCaseCounts(results)
	minSamples, maxSamples := sampleRange(results)
	infraErrors := 0
	for _, r := range results {
		if r.Error != "" {
			infraErrors++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "DIAGNOSIS\n\n")
	fmt.Fprintf(&b, "Run health\n")
	fmt.Fprintf(&b, "  %d samples, %d models, %d cases, %d infrastructure errors", len(results), len(models), cases, infraErrors)
	if runIDs > 1 {
		fmt.Fprintf(&b, ", %d runs", runIDs)
	}
	b.WriteString(".\n")
	if minSamples == maxSamples {
		fmt.Fprintf(&b, "  Evidence: %d sample(s) per model/case", minSamples)
	} else {
		fmt.Fprintf(&b, "  Evidence: %d-%d samples per model/case", minSamples, maxSamples)
	}
	if minSamples < 3 {
		b.WriteString("; rankings are directional. Repeat finalists at least 3 times.\n")
	} else {
		b.WriteString(".\n")
	}
	writeInstability(&b, results)

	writeRecommendations(&b, models)
	writeBenchmarkSignals(&b, results)
	writeFailureModes(&b, results)
	writeModelSummary(&b, models)
	return b.String()
}

func aggregateModels(results []bench.Result) []*modelDiagnosis {
	byName := map[string]*modelDiagnosis{}
	for _, r := range results {
		m := byName[r.Model]
		if m == nil {
			m = &modelDiagnosis{name: r.Model, caseIDs: map[string]bool{}}
			byName[r.Model] = m
		}
		m.n++
		m.caseIDs[r.CaseID] = true
		m.score += r.Score
		if r.Passed {
			m.passes++
		}
		if r.Error != "" {
			m.errors++
		} else {
			m.completed++
			m.wallSeconds += float64(r.Metrics.WallDurationMS) / 1000
			if r.Metrics.OutputTokensPerSec > 0 {
				m.decodeTPS = append(m.decodeTPS, r.Metrics.OutputTokensPerSec)
			}
		}
		switch r.Category {
		case "classification":
			m.classificationN++
			m.classificationScore += r.Score
			if r.FormatPassed != nil {
				m.formatN++
				if *r.FormatPassed {
					m.formatPass++
				}
			}
		case "coding":
			m.codingN++
			if r.Passed {
				m.codingPass++
			}
		}
	}
	models := make([]*modelDiagnosis, 0, len(byName))
	for _, m := range byName {
		models = append(models, m)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].name < models[j].name })
	return models
}

func uniqueRunAndCaseCounts(results []bench.Result) (int, int) {
	runs := map[string]bool{}
	cases := map[string]bool{}
	for _, r := range results {
		runs[r.RunID] = true
		cases[r.CaseID] = true
	}
	return len(runs), len(cases)
}

func sampleRange(results []bench.Result) (int, int) {
	counts := map[string]int{}
	for _, r := range results {
		counts[r.Model+"\x00"+r.CaseID]++
	}
	minN, maxN := 0, 0
	for _, n := range counts {
		if minN == 0 || n < minN {
			minN = n
		}
		if n > maxN {
			maxN = n
		}
	}
	return minN, maxN
}

func writeRecommendations(b *strings.Builder, models []*modelDiagnosis) {
	if len(models) == 0 {
		return
	}
	quality := append([]*modelDiagnosis(nil), models...)
	sort.SliceStable(quality, func(i, j int) bool {
		if quality[i].quality() != quality[j].quality() {
			return quality[i].quality() > quality[j].quality()
		}
		if quality[i].suiteWallSeconds() != quality[j].suiteWallSeconds() {
			return quality[i].suiteWallSeconds() < quality[j].suiteWallSeconds()
		}
		return quality[i].name < quality[j].name
	})
	qualityLeader := quality[0]

	speed := append([]*modelDiagnosis(nil), models...)
	sort.SliceStable(speed, func(i, j int) bool {
		if speed[i].suiteWallSeconds() != speed[j].suiteWallSeconds() {
			return speed[i].suiteWallSeconds() < speed[j].suiteWallSeconds()
		}
		return speed[i].name < speed[j].name
	})
	speedLeader := speed[0]

	balanced := qualityLeader
	qualityFloor := qualityLeader.quality() * balancedQualityFraction
	for _, m := range models {
		if m.quality() >= qualityFloor && m.suiteWallSeconds() < balanced.suiteWallSeconds() {
			balanced = m
		}
	}

	var formatLeader *modelDiagnosis
	for _, m := range models {
		if m.formatN == 0 {
			continue
		}
		if formatLeader == nil || formatRate(m) > formatRate(formatLeader) ||
			(formatRate(m) == formatRate(formatLeader) && m.quality() > formatLeader.quality()) {
			formatLeader = m
		}
	}

	b.WriteString("\nRecommendations\n")
	b.WriteString("  Rule: quality is mean case score; wall time is normalized to one pass over the observed cases.\n")
	fmt.Fprintf(b, "  Quality: %s (quality %.3f, coding %d/%d, classification %.3f).\n", qualityLeader.name, qualityLeader.quality(), qualityLeader.codingPass, qualityLeader.codingN, qualityLeader.classificationQuality())
	fmt.Fprintf(b, "  Balanced: %s (quality %.3f in %s); lowest wall time within 85%% of the quality leader.\n", balanced.name, balanced.quality(), duration(balanced.suiteWallSeconds()))
	fmt.Fprintf(b, "  Throughput: %s (%s per suite, median decode %.1f tok/s; quality %.3f).\n", speedLeader.name, duration(speedLeader.suiteWallSeconds()), median(speedLeader.decodeTPS), speedLeader.quality())
	if formatLeader != nil {
		fmt.Fprintf(b, "  Structured output: %s (%d/%d exact-format responses).\n", formatLeader.name, formatLeader.formatPass, formatLeader.formatN)
	}
	frontier := paretoFrontier(models)
	names := make([]string, 0, len(frontier))
	for _, m := range frontier {
		names = append(names, m.name)
	}
	fmt.Fprintf(b, "  Quality/wall-time frontier: %s.\n", strings.Join(names, ", "))
	finalists := uniqueModels(qualityLeader, balanced, formatLeader)
	if len(finalists) < 3 {
		finalists = uniqueModels(append(finalists, speedLeader)...)
	}
	finalistNames := make([]string, 0, len(finalists))
	estimatedSeconds := 0.0
	for _, m := range finalists {
		finalistNames = append(finalistNames, m.name)
		estimatedSeconds += m.suiteWallSeconds() * 3
	}
	fmt.Fprintf(b, "  Suggested repeat: ./bin/lme run --repeats 3 --models %s\n", strings.Join(finalistNames, ","))
	fmt.Fprintf(b, "  Estimated repeat wall time: about %s, based on this run.\n", duration(estimatedSeconds))
}

func uniqueModels(models ...*modelDiagnosis) []*modelDiagnosis {
	seen := map[string]bool{}
	var unique []*modelDiagnosis
	for _, model := range models {
		if model == nil || seen[model.name] {
			continue
		}
		seen[model.name] = true
		unique = append(unique, model)
	}
	return unique
}

func paretoFrontier(models []*modelDiagnosis) []*modelDiagnosis {
	var frontier []*modelDiagnosis
	for _, candidate := range models {
		dominated := false
		for _, other := range models {
			if other == candidate {
				continue
			}
			atLeastAsGood := other.quality() >= candidate.quality() && other.suiteWallSeconds() <= candidate.suiteWallSeconds()
			strictlyBetter := other.quality() > candidate.quality() || other.suiteWallSeconds() < candidate.suiteWallSeconds()
			if atLeastAsGood && strictlyBetter {
				dominated = true
				break
			}
		}
		if !dominated {
			frontier = append(frontier, candidate)
		}
	}
	sort.Slice(frontier, func(i, j int) bool {
		if frontier[i].suiteWallSeconds() != frontier[j].suiteWallSeconds() {
			return frontier[i].suiteWallSeconds() < frontier[j].suiteWallSeconds()
		}
		return frontier[i].name < frontier[j].name
	})
	return frontier
}

func formatRate(m *modelDiagnosis) float64 {
	if m.formatN == 0 {
		return 0
	}
	return float64(m.formatPass) / float64(m.formatN)
}

func writeBenchmarkSignals(b *strings.Builder, results []bench.Result) {
	type caseStats struct {
		category string
		n, pass  int
		score    float64
	}
	byCase := map[string]*caseStats{}
	formatN, formatPass := 0, 0
	for _, r := range results {
		s := byCase[r.CaseID]
		if s == nil {
			s = &caseStats{category: r.Category}
			byCase[r.CaseID] = s
		}
		s.n++
		s.score += r.Score
		if r.Passed {
			s.pass++
		}
		if r.FormatPassed != nil {
			formatN++
			if *r.FormatPassed {
				formatPass++
			}
		}
	}

	b.WriteString("\nBenchmark signals\n")
	var noDiscrimination, unanimousFailure []string
	for id, s := range byCase {
		if s.pass == s.n {
			noDiscrimination = append(noDiscrimination, id)
		}
		if s.pass == 0 {
			unanimousFailure = append(unanimousFailure, id)
		}
	}
	sort.Strings(noDiscrimination)
	sort.Strings(unanimousFailure)
	if len(noDiscrimination) > 0 {
		fmt.Fprintf(b, "  No discrimination (all passed): %s.\n", strings.Join(noDiscrimination, ", "))
	}
	if len(unanimousFailure) > 0 {
		fmt.Fprintf(b, "  Unanimous failures: %s. Review difficulty and failure evidence; do not assume the scorer is wrong.\n", strings.Join(unanimousFailure, ", "))
	}
	if formatN > 0 {
		fmt.Fprintf(b, "  Exact-format compliance: %d/%d (%.0f%%); semantic and schema reliability differ materially.\n", formatPass, formatN, 100*float64(formatPass)/float64(formatN))
	}
	writeConsensusMistakes(b, results)
}

type semanticMistake struct {
	caseID, artifact, predicted, expected string
	count, samples                        int
}

func writeConsensusMistakes(b *strings.Builder, results []bench.Result) {
	type key struct{ caseID, artifact, predicted, expected string }
	counts := map[key]int{}
	caseSamples := map[string]int{}
	for _, r := range results {
		if r.Category != "classification" {
			continue
		}
		caseSamples[r.CaseID]++
		for _, m := range parseSemanticMistakes(r.Details) {
			counts[key{r.CaseID, m.artifact, m.predicted, m.expected}]++
		}
	}
	var mistakes []semanticMistake
	for k, count := range counts {
		samples := caseSamples[k.caseID]
		if samples > 1 && count*2 >= samples {
			mistakes = append(mistakes, semanticMistake{k.caseID, k.artifact, k.predicted, k.expected, count, samples})
		}
	}
	sort.Slice(mistakes, func(i, j int) bool {
		if mistakes[i].count != mistakes[j].count {
			return mistakes[i].count > mistakes[j].count
		}
		if mistakes[i].caseID != mistakes[j].caseID {
			return mistakes[i].caseID < mistakes[j].caseID
		}
		return mistakes[i].artifact < mistakes[j].artifact
	})
	for _, m := range mistakes {
		fmt.Fprintf(b, "  Consensus semantic miss: %s/%s predicted %s, expected %s (%d/%d samples).\n", m.caseID, m.artifact, m.predicted, m.expected, m.count, m.samples)
	}
}

func parseSemanticMistakes(details string) []semanticMistake {
	var out []semanticMistake
	for _, field := range strings.Fields(details) {
		eq := strings.IndexByte(field, '=')
		want := strings.Index(field, "(want:")
		if eq <= 0 || want <= eq || !strings.HasSuffix(field, ")") {
			continue
		}
		predicted := field[eq+1 : want]
		expected := field[want+6 : len(field)-1]
		if predicted != expected {
			out = append(out, semanticMistake{artifact: field[:eq], predicted: predicted, expected: expected})
		}
	}
	return out
}

func writeFailureModes(b *strings.Builder, results []bench.Result) {
	counts := map[string]int{}
	totalFailures := 0
	claimedSuccess := 0
	for _, r := range results {
		if r.Category != "coding" || r.Passed {
			continue
		}
		totalFailures++
		counts[codingFailureMode(r)]++
		if claimedTestsPassed(r) {
			claimedSuccess++
		}
	}
	if totalFailures == 0 {
		return
	}
	type pair struct {
		name  string
		count int
	}
	var modes []pair
	for name, count := range counts {
		modes = append(modes, pair{name, count})
	}
	sort.Slice(modes, func(i, j int) bool {
		if modes[i].count != modes[j].count {
			return modes[i].count > modes[j].count
		}
		return modes[i].name < modes[j].name
	})
	b.WriteString("\nCoding failure modes\n")
	for _, mode := range modes {
		fmt.Fprintf(b, "  %d/%d: %s.\n", mode.count, totalFailures, mode.name)
	}
	if claimedSuccess > 0 {
		fmt.Fprintf(b, "  %d failed solution(s) claimed tests passed; hidden verification correctly caught overconfidence.\n", claimedSuccess)
	}
}

func codingFailureMode(r bench.Result) string {
	if r.Error != "" {
		return "infrastructure failure"
	}
	wrote := false
	for _, entry := range r.Transcript {
		for _, call := range entry.ToolCalls {
			if call.Function.Name == "write_file" {
				wrote = true
			}
		}
		if entry.ToolName == "write_file" {
			wrote = true
		}
	}
	lower := strings.ToLower(r.Details)
	if !wrote {
		return "no implementation attempted"
	}
	if strings.Contains(lower, "undefined:") || strings.Contains(lower, "build failed") || strings.Contains(lower, "cannot compile") {
		return "implementation did not compile"
	}
	if strings.Contains(lower, "panic: todo") {
		return "TODO remained in the implementation"
	}
	if strings.Contains(lower, "len ") || strings.Contains(lower, "length") {
		return "incomplete output or missing records"
	}
	if strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timed out") {
		return "verification timed out"
	}
	return "hidden behavioral edge case failed"
}

func claimedTestsPassed(r bench.Result) bool {
	for i := len(r.Transcript) - 1; i >= 0; i-- {
		entry := r.Transcript[i]
		if entry.Role != "assistant" || strings.TrimSpace(entry.Content) == "" {
			continue
		}
		lower := strings.ToLower(entry.Content)
		return strings.Contains(lower, "tests pass") || strings.Contains(lower, "all tests pass") || strings.Contains(lower, "tests are passing")
	}
	return false
}

func writeInstability(b *strings.Builder, results []bench.Result) {
	type outcome struct{ pass, fail bool }
	by := map[string]*outcome{}
	for _, r := range results {
		key := r.Model + " / " + r.CaseID
		o := by[key]
		if o == nil {
			o = &outcome{}
			by[key] = o
		}
		if r.Passed {
			o.pass = true
		} else {
			o.fail = true
		}
	}
	var unstable []string
	for key, o := range by {
		if o.pass && o.fail {
			unstable = append(unstable, key)
		}
	}
	if len(unstable) == 0 {
		return
	}
	sort.Strings(unstable)
	fmt.Fprintf(b, "  Unstable outcomes across repeats (%d): %s.\n", len(unstable), strings.Join(unstable, "; "))
}

func writeModelSummary(b *strings.Builder, models []*modelDiagnosis) {
	sorted := append([]*modelDiagnosis(nil), models...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].quality() != sorted[j].quality() {
			return sorted[i].quality() > sorted[j].quality()
		}
		return sorted[i].name < sorted[j].name
	})
	b.WriteString("\nModel summary\n")
	for _, m := range sorted {
		format := "-"
		if m.formatN > 0 {
			format = fmt.Sprintf("%d/%d", m.formatPass, m.formatN)
		}
		fmt.Fprintf(b, "  %-38s quality %.3f  coding %d/%d  class %.3f  format %s  wall %s  decode %.1f\n",
			m.name, m.quality(), m.codingPass, m.codingN, m.classificationQuality(), format, duration(m.suiteWallSeconds()), median(m.decodeTPS))
	}
}

func duration(seconds float64) string {
	total := int64(seconds + 0.5)
	if total < 60 {
		return strconv.FormatInt(total, 10) + "s"
	}
	return fmt.Sprintf("%dm%02ds", total/60, total%60)
}
