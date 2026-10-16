package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
	"github.com/example/local-model-eval/internal/telemetry"
)

type codingCase struct {
	ID, Category, Difficulty, Task string
	VisibleTestCommand             []string `json:"visible_test_command"`
	VerifyCommand                  []string `json:"verify_command"`
	// GradeRun, when set, is the -run filter used to grade only the hidden
	// grader tests verbosely, enabling per-artifact partial credit. The model's
	// own run_tests still runs the full visible suite, so it never sees the
	// grader it is scored on.
	GradeRun string `json:"grade_run"`
}

// codingNudgeLimit caps how many times a no-tool-call turn is interrupted with a
// request to actually implement before the run is treated as finished. The nudge
// only fires when the model has never written a file, so a model that already
// produced a candidate solution and then went quiet is a real completion, not a
// give-up.
const codingNudgeLimit = 2

func codingNudge() string {
	return "You have inspected the repository but have not edited any file yet. Implement the fix now: call write_file to create or replace the target source file, then call run_tests. Respond with a tool call, not prose."
}

func codingTools() []ollama.Tool {
	return []ollama.Tool{
		{Type: "function", Function: ollama.ToolFunction{Name: "list_files", Description: "List files in the benchmark repository.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
		{Type: "function", Function: ollama.ToolFunction{Name: "read_file", Description: "Read a UTF-8 text file from the benchmark repository.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}}},
		{Type: "function", Function: ollama.ToolFunction{Name: "write_file", Description: "Create or replace a UTF-8 text file in the benchmark repository. Use this to implement the fix.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"path", "content"}}}},
		{Type: "function", Function: ollama.ToolFunction{Name: "run_tests", Description: "Run the benchmark's visible test command. Use after making changes.", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}}},
	}
}

func (r *Runner) runCoding(ctx context.Context, model, casePath string) (Result, error) {
	var c codingCase
	b, err := os.ReadFile(casePath)
	if err != nil {
		return Result{}, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return Result{}, err
	}
	if c.ID == "" || c.Category != "coding" || c.Task == "" || len(c.VisibleTestCommand) == 0 || len(c.VerifyCommand) == 0 {
		return Result{}, fmt.Errorf("%s: invalid coding case", casePath)
	}
	caseDir := filepath.Dir(casePath)
	tmp, err := os.MkdirTemp("", "lme-coding-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp)
	if err = copyTree(filepath.Join(caseDir, "fixture"), tmp); err != nil {
		return Result{}, err
	}
	res := r.baseResult(ctx, model, c.ID, c.Category, c.Difficulty)
	before := telemetry.Capture()
	totalStart := time.Now()
	messages := []ollama.Message{{Role: "system", Content: "You are solving a coding benchmark in an isolated repository. Inspect the repository using tools, make the smallest correct change, and run visible tests. Do not merely explain a solution: edit the files. You cannot access files outside the repository."}, {Role: "user", Content: c.Task}}
	transcript := []TranscriptEntry{
		{Step: 0, Role: "system", Content: messages[0].Content},
		{Step: 0, Role: "user", Content: messages[1].Content},
	}
	var agg Metrics
	steps := 0
	wrote := false
	nudgesUsed := 0
	var transcriptTail string
	for steps < r.Config.MaxAgentSteps {
		steps++
		start := time.Now()
		resp, e := r.Client.Chat(ctx, ollama.ChatRequest{Model: model, Messages: messages, Tools: codingTools(), Stream: false, Options: options(r.Config)})
		addMetrics(&agg, resp, time.Since(start))
		if e != nil {
			agg.WallDurationMS = time.Since(totalStart).Milliseconds()
			res.AgentSteps = steps
			res.Metrics = agg
			res.Before = before
			res.After = telemetry.Capture()
			res.Transcript = transcript
			res.Details = trimDetails("assistant_final:\n"+transcriptTail+"\ninfrastructure_error: "+e.Error(), 3000)
			return res, e
		}
		messages = append(messages, resp.Message)
		transcript = append(transcript, TranscriptEntry{Step: steps, Role: "assistant", Content: resp.Message.Content, Thinking: resp.Message.Thinking, ToolCalls: resp.Message.ToolCalls})
		transcriptTail = resp.Message.Content
		if len(resp.Message.ToolCalls) == 0 {
			// The model stopped acting. If it never wrote a file, this is most
			// likely "I'll implement it next" rather than a real completion: nudge
			// it to act before treating the run as finished. A model that already
			// wrote and then went quiet has produced a candidate solution, so we
			// let that stand and break.
			if !wrote && nudgesUsed < codingNudgeLimit {
				nudge := codingNudge()
				messages = append(messages, ollama.Message{Role: "user", Content: nudge})
				transcript = append(transcript, TranscriptEntry{Step: steps, Role: "user", Content: nudge})
				nudgesUsed++
				continue
			}
			break
		}
		for _, tc := range resp.Message.ToolCalls {
			if tc.Function.Name == "write_file" {
				wrote = true
			}
			out := r.execCodingTool(ctx, tmp, c, tc)
			messages = append(messages, ollama.Message{Role: "tool", Content: out})
			transcript = append(transcript, TranscriptEntry{Step: steps, Role: "tool", Content: out, ToolName: tc.Function.Name, Arguments: tc.Function.Arguments})
		}
	}
	res.Transcript = transcript
	// Hidden verifier becomes visible only after the model is done.
	if hidden := filepath.Join(caseDir, "hidden"); dirExists(hidden) {
		if err := copyTree(hidden, tmp); err != nil {
			return res, err
		}
	}
	// Grade. When the case declares a grade_run, grade only the hidden grader
	// tests verbosely so we can award per-artifact partial credit; otherwise run
	// the verifier as-is and fall back to a binary pass/fail.
	gradeCmd := c.VerifyCommand
	if c.GradeRun != "" {
		gradeCmd = graderCommand(c.VerifyCommand, c.GradeRun)
	}
	verifyOut, verifyErr := runCommand(ctx, tmp, gradeCmd, time.Duration(r.Config.TestTimeoutSeconds)*time.Second)
	agg.WallDurationMS = time.Since(totalStart).Milliseconds()
	after := telemetry.Capture()
	if errors.Is(verifyErr, context.DeadlineExceeded) || errors.Is(verifyErr, context.Canceled) {
		res.Error = verifyErr.Error()
	}
	if passed, total, ok := parseSubtests(verifyOut); ok {
		// Per-artifact partial credit from the verbose subtest output.
		res.Score = float64(passed) / float64(total)
		res.Passed = passed == total
		verifyOut = fmt.Sprintf("subtests: %d/%d passed\n", passed, total) + verifyOut
	} else {
		// No subtest output (build failure, a trivial verifier, or a non-subtest
		// grader): keep the original binary semantics.
		res.Passed = verifyErr == nil
		if res.Passed {
			res.Score = 1
		}
	}
	res.AgentSteps = steps
	res.Metrics = agg
	res.Before = before
	res.After = after
	res.Transcript = transcript
	res.Details = trimDetails("verifier:\n"+verifyOut+"\nassistant_final:\n"+transcriptTail, 3000)
	return res, nil
}

func (r *Runner) execCodingTool(ctx context.Context, root string, c codingCase, tc ollama.ToolCall) string {
	switch tc.Function.Name {
	case "list_files":
		var files []string
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(root, path)
				if !strings.HasPrefix(rel, ".git/") {
					files = append(files, rel)
				}
			}
			return nil
		})
		sort.Strings(files)
		return strings.Join(files, "\n")
	case "read_file":
		rel, _ := tc.Function.Arguments["path"].(string)
		p, err := safePath(root, rel)
		if err != nil {
			return "ERROR: " + err.Error()
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "ERROR: " + err.Error()
		}
		if len(b) > 200000 {
			return "ERROR: file too large"
		}
		return string(b)
	case "write_file":
		rel, _ := tc.Function.Arguments["path"].(string)
		content, _ := tc.Function.Arguments["content"].(string)
		p, err := safePath(root, rel)
		if err != nil {
			return "ERROR: " + err.Error()
		}
		if err = os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "ERROR: " + err.Error()
		}
		if err = os.WriteFile(p, []byte(content), 0o644); err != nil {
			return "ERROR: " + err.Error()
		}
		return "OK"
	case "run_tests":
		out, err := runCommand(ctx, root, c.VisibleTestCommand, time.Duration(r.Config.TestTimeoutSeconds)*time.Second)
		if err != nil {
			return "FAIL\n" + trimDetails(out, 8000)
		}
		return "PASS\n" + trimDetails(out, 8000)
	default:
		return fmt.Sprintf("ERROR: unsupported tool %q", tc.Function.Name)
	}
}

func dirExists(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }
func trimDetails(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// graderCommand turns a plain `go test ./...` verifier into a verbose, filtered
// run (`go test -v -run <gradeRun> ./...`) so the grader's subtests emit the
// --- PASS/FAIL lines parseSubtests counts. Non-`go test` verifiers pass through
// unchanged.
func graderCommand(verify []string, gradeRun string) []string {
	if len(verify) < 2 || verify[0] != "go" || verify[1] != "test" {
		return verify
	}
	out := []string{"go", "test", "-v", "-run", gradeRun}
	for _, a := range verify[2:] {
		out = append(out, a)
	}
	return out
}

// parseSubtests counts the --- PASS/FAIL lines of a `go test -v` run. Only
// subtests (test names containing "/") are counted, so a parent test and its
// children are not double-counted and a grader with no subtests yields ok=false,
// signaling the caller to fall back to a binary pass/fail.
func parseSubtests(output string) (passed, total int, ok bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		var status string
		switch {
		case strings.HasPrefix(line, "--- PASS:"):
			status = "PASS"
		case strings.HasPrefix(line, "--- FAIL:"):
			status = "FAIL"
		default:
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, status+":"))
		if i := strings.Index(name, " ("); i >= 0 {
			name = strings.TrimSpace(name[:i])
		}
		if !strings.Contains(name, "/") {
			continue
		}
		total++
		if status == "PASS" {
			passed++
		}
	}
	return passed, total, total > 0
}
