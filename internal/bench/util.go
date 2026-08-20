package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func LoadConfig(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return c, fmt.Errorf("configuration must contain exactly one JSON object")
		}
		return c, err
	}
	if c.OllamaURL == "" {
		return c, fmt.Errorf("ollama_url must not be empty")
	}
	if c.ContextSize <= 0 || c.MaxAgentSteps <= 0 || c.MaxOutputTokens <= 0 {
		return c, fmt.Errorf("context_size, max_agent_steps, and max_output_tokens must be positive")
	}
	if c.ChatTimeoutSeconds <= 0 || c.TestTimeoutSeconds <= 0 {
		return c, fmt.Errorf("chat_timeout_seconds and test_timeout_seconds must be positive")
	}
	return c, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			_ = in.Close()
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, cpErr := io.Copy(out, in)
		inErr := in.Close()
		closeErr := out.Close()
		if cpErr != nil {
			return cpErr
		}
		if inErr != nil {
			return inErr
		}
		return closeErr
	})
}

func safePath(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	p := filepath.Clean(filepath.Join(root, rel))
	r, _ := filepath.Abs(root)
	a, _ := filepath.Abs(p)
	if a != r && !strings.HasPrefix(a, r+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes sandbox")
	}
	return a, nil
}

func runCommand(parent context.Context, dir string, argv []string, timeout time.Duration) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("empty command")
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(b), fmt.Errorf("command stopped: %w", ctx.Err())
	}
	return string(b), err
}

func OllamaVersion() string {
	b, err := exec.Command("ollama", "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// InstalledModels returns the model names reported by the local Ollama CLI.
func InstalledModels() ([]string, error) {
	b, err := exec.Command("ollama", "list").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ollama list: %w: %s", err, strings.TrimSpace(string(b)))
	}
	models := parseOllamaList(string(b))
	if len(models) == 0 {
		return nil, fmt.Errorf("ollama list returned no installed models")
	}
	return models, nil
}

func parseOllamaList(output string) []string {
	var models []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.EqualFold(fields[0], "NAME") {
			continue
		}
		models = append(models, fields[0])
	}
	return models
}
