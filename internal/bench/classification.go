package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/example/local-model-eval/internal/ollama"
	"github.com/example/local-model-eval/internal/telemetry"
)

type classificationCase struct {
	ID          string            `json:"id"`
	Category    string            `json:"category"`
	Description string            `json:"description"`
	Projects    []map[string]any  `json:"projects"`
	Artifacts   []map[string]any  `json:"artifacts"`
	Expected    map[string]string `json:"expected"`
}

type assignmentResponse struct {
	Assignments []assignment `json:"assignments"`
}

type assignment struct {
	ArtifactID string `json:"artifact_id"`
	ProjectID  string `json:"project_id"`
}

func (r *Runner) runClassification(ctx context.Context, model, path string) (Result, error) {
	var c classificationCase
	b, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return Result{}, err
	}
	artifactIDs, projectIDs, err := validateClassificationCase(c)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", path, err)
	}
	input, _ := json.MarshalIndent(map[string]any{"projects": c.Projects, "artifacts": c.Artifacts}, "", "  ")
	prompt := fmt.Sprintf("%s\nAssign every artifact to exactly one project id, or 'unrelated'. Use semantic evidence across people, topics, codes, and wording; do not force weak matches. Return only the requested structured data.\n\n%s", c.Description, string(input))
	schema := classificationSchema(artifactIDs, projectIDs)
	res := r.baseResult(ctx, model, c.ID, c.Category, "")
	before := telemetry.Capture()
	start := time.Now()
	resp, err := r.Client.Chat(ctx, ollama.ChatRequest{Model: model, Messages: []ollama.Message{{Role: "user", Content: prompt}}, Format: schema, Stream: false, Options: options(r.Config)})
	wall := time.Since(start)
	after := telemetry.Capture()
	res.Metrics = metricsFrom(resp, wall)
	res.Before = before
	res.After = after
	if err != nil {
		return res, err
	}
	res.RawResponse = resp.Message.Content
	outcome := scoreClassification(c, resp.Message.Content)
	res.Score = outcome.SemanticScore
	res.Passed = outcome.SemanticPassed
	res.FormatPassed = &outcome.FormatPassed
	res.Details = outcome.Details
	return res, nil
}

type classificationOutcome struct {
	SemanticScore  float64
	SemanticPassed bool
	FormatPassed   bool
	Details        string
}

func scoreClassification(c classificationCase, response string) classificationOutcome {
	semanticAssignments := extractSemanticAssignments(response)
	predictions := make(map[string]map[string]bool)
	for _, a := range semanticAssignments {
		if predictions[a.ArtifactID] == nil {
			predictions[a.ArtifactID] = map[string]bool{}
		}
		predictions[a.ArtifactID][a.ProjectID] = true
	}

	correct := 0
	for id, want := range c.Expected {
		projects := predictions[id]
		if len(projects) == 1 && projects[want] {
			correct++
		}
	}
	semanticScore := float64(correct) / float64(len(c.Expected))
	_, formatErr := strictAssignments(c, response)

	keys := make([]string, 0, len(c.Expected))
	for k := range c.Expected {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	details := ""
	for _, k := range keys {
		projects := make([]string, 0, len(predictions[k]))
		for project := range predictions[k] {
			projects = append(projects, project)
		}
		sort.Strings(projects)
		details += fmt.Sprintf("%s=%s(want:%s) ", k, strings.Join(projects, "|"), c.Expected[k])
	}
	if len(semanticAssignments) == 0 {
		details += "semantic_error=no recognizable assignments "
	}
	if formatErr != nil {
		details += "format_error=" + formatErr.Error()
	}
	return classificationOutcome{
		SemanticScore:  semanticScore,
		SemanticPassed: correct == len(c.Expected),
		FormatPassed:   formatErr == nil,
		Details:        strings.TrimSpace(details),
	}
}

func strictAssignments(c classificationCase, response string) ([]assignment, error) {
	dec := json.NewDecoder(strings.NewReader(response))
	dec.DisallowUnknownFields()
	var got assignmentResponse
	if err := dec.Decode(&got); err != nil {
		return nil, fmt.Errorf("invalid requested JSON: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("trailing content: %w", err)
	}
	validProjects := map[string]bool{"unrelated": true}
	for _, project := range c.Projects {
		id, _ := project["id"].(string)
		validProjects[id] = true
	}
	seen := map[string]bool{}
	var problems []string
	for _, a := range got.Assignments {
		if _, ok := c.Expected[a.ArtifactID]; !ok {
			problems = append(problems, fmt.Sprintf("unknown artifact %q", a.ArtifactID))
		}
		if seen[a.ArtifactID] {
			problems = append(problems, fmt.Sprintf("duplicate artifact %q", a.ArtifactID))
		}
		if !validProjects[a.ProjectID] {
			problems = append(problems, fmt.Sprintf("unknown project %q", a.ProjectID))
		}
		seen[a.ArtifactID] = true
	}
	if len(got.Assignments) != len(c.Expected) {
		problems = append(problems, fmt.Sprintf("got %d assignments, want %d", len(got.Assignments), len(c.Expected)))
	}
	keys := make([]string, 0, len(c.Expected))
	for id := range c.Expected {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if !seen[id] {
			problems = append(problems, fmt.Sprintf("missing artifact %q", id))
		}
	}
	if len(problems) > 0 {
		return got.Assignments, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return got.Assignments, nil
}

func extractSemanticAssignments(response string) []assignment {
	data := []byte(response)
	for i, b := range data {
		if b != '{' && b != '[' {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(data[i:]))
		var value any
		if err := dec.Decode(&value); err != nil {
			continue
		}
		if assignments := assignmentsFromValue(value); len(assignments) > 0 {
			return assignments
		}
	}
	return nil
}

func assignmentsFromValue(value any) []assignment {
	switch v := value.(type) {
	case map[string]any:
		if nested, ok := v["assignments"]; ok {
			return assignmentsFromValue(nested)
		}
		artifactID := firstString(v, "artifact_id", "artifactId", "artifactID", "artifact", "id")
		projectID := firstString(v, "project_id", "projectId", "projectID", "project")
		if artifactID != "" && projectID != "" {
			return []assignment{{ArtifactID: artifactID, ProjectID: projectID}}
		}
		var out []assignment
		for artifactID, rawProject := range v {
			if projectID, ok := rawProject.(string); ok {
				out = append(out, assignment{ArtifactID: artifactID, ProjectID: projectID})
			}
		}
		return out
	case []any:
		var out []assignment
		for _, item := range v {
			out = append(out, assignmentsFromValue(item)...)
		}
		return out
	}
	return nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			return value
		}
	}
	return ""
}

func validateClassificationCase(c classificationCase) ([]string, []string, error) {
	if c.ID == "" || c.Category != "classification" || len(c.Artifacts) == 0 || len(c.Expected) == 0 {
		return nil, nil, fmt.Errorf("missing id, artifacts, or expected assignments")
	}
	artifactIDs := make([]string, 0, len(c.Artifacts))
	seenArtifacts := map[string]bool{}
	for _, artifact := range c.Artifacts {
		id, _ := artifact["id"].(string)
		if id == "" || seenArtifacts[id] {
			return nil, nil, fmt.Errorf("artifact ids must be non-empty and unique")
		}
		seenArtifacts[id] = true
		artifactIDs = append(artifactIDs, id)
		if _, ok := c.Expected[id]; !ok {
			return nil, nil, fmt.Errorf("artifact %q has no expected assignment", id)
		}
	}
	if len(c.Expected) != len(artifactIDs) {
		return nil, nil, fmt.Errorf("expected assignments do not match artifacts")
	}
	projectIDs := []string{"unrelated"}
	seenProjects := map[string]bool{"unrelated": true}
	for _, project := range c.Projects {
		id, _ := project["id"].(string)
		if id == "" || seenProjects[id] {
			return nil, nil, fmt.Errorf("project ids must be non-empty, unique, and not 'unrelated'")
		}
		seenProjects[id] = true
		projectIDs = append(projectIDs, id)
	}
	for artifactID, projectID := range c.Expected {
		if !seenProjects[projectID] {
			return nil, nil, fmt.Errorf("artifact %q expects unknown project %q", artifactID, projectID)
		}
	}
	return artifactIDs, projectIDs, nil
}

func classificationSchema(artifactIDs, projectIDs []string) map[string]any {
	item := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"artifact_id": map[string]any{"type": "string", "enum": artifactIDs},
			"project_id":  map[string]any{"type": "string", "enum": projectIDs},
		},
		"required":             []string{"artifact_id", "project_id"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"assignments": map[string]any{"type": "array", "items": item, "minItems": len(artifactIDs), "maxItems": len(artifactIDs)},
		},
		"required":             []string{"assignments"},
		"additionalProperties": false,
	}
}
