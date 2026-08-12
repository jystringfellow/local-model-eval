# local-model-eval

A small, repeatable benchmark harness for comparing local Ollama models on **your actual machine**.

It answers four questions separately rather than collapsing everything into one opaque score:

1. **Coding correctness** — can the model inspect a repo, edit files, run tests, and satisfy hidden tests?
2. **Work-context intelligence** — can it classify and link Slack/email/commit/meeting artifacts to the right project?
3. **General assistant reliability** — can it follow concrete instructions and produce useful structured answers?
4. **Machine efficiency** — how fast does it prefill/decode, how long does a task take, and what happens to memory/swap?

The harness talks directly to Ollama's local `/api/chat` API. Coding tasks use a restricted tool loop in a disposable temp directory. Hidden tests are copied in **only after the model finishes**, so the score is deterministic and the verifier is not visible to the model.

## Requirements

- macOS or Linux
- Go 1.23+
- Ollama running locally
- the models in `bench.json` already pulled

## Quick start

```sh
make build
./bin/lme doctor
./bin/lme list
./bin/lme run --models qwen3.6:35b-mlx,north-mini-code-1.0:mlx-nvfp4
./bin/lme report
```

Run only a category:

```sh
./bin/lme run --category coding --models qwen3.6:35b-mlx
./bin/lme run --category classification --models gemma4:12b-mlx,qwen3.6:35b-mlx
```

Results are appended to `results/results.jsonl`. `lme report` prints a compact leaderboard based on pass rate plus speed metrics; it intentionally keeps category scores separate.

## Adding a coding benchmark

A coding task directory contains:

```text
_cases/coding/my-task/
  case.json
  fixture/        # repo visible to the model
  hidden/         # verifier files NOT visible until scoring
```

`case.json` describes the task and test command. The model gets four tools scoped to a temporary copy of `fixture/`: `list_files`, `read_file`, `write_file`, and `run_tests`. After the agent loop ends, the harness copies `hidden/` into the temp repo and executes the verifier.

Prefer small, self-contained fixtures with strong hidden tests. Keep network access unnecessary.

## Adding Workgraph-style classification cases

Classification cases are JSON files in `_cases/classification/`. Each case provides project definitions, mixed artifacts, and expected project assignments. The model must return structured JSON, and the harness scores exact artifact-to-project accuracy.

Good cases include:

- explicit names and ticket IDs (easy)
- indirect references and people overlap (medium)
- misleading keywords / unrelated artifacts (hard)
- multiple projects sharing a customer or technology (hard)

## Benchmark hygiene

For meaningful comparisons:

- use the same `context_size`, temperature, cases, and Ollama version;
- run each model multiple times when comparing close results;
- use medians for speed;
- don't compare results across major harness changes without noting the benchmark git SHA;
- treat a model that causes swap or makes the machine unusable as worse even if its raw correctness is slightly higher.

## Safety

Coding tasks execute model-edited code. The harness restricts file tools to a temp directory and does not expose arbitrary shell commands to the model, but the configured test command itself executes locally. Only add benchmark fixtures you trust. For untrusted third-party tasks, run the harness inside a VM/container.
