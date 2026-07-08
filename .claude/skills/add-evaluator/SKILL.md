---
name: add-evaluator
description: Add a new evaluator to the amp-evaluation Python library. Use when the user asks to add, write, or register an evaluator, LLM-as-judge, or scoring check for agent traces in libs/amp-evaluation. Covers the decorator API and the type-hint-driven level/mode detection that determines whether the evaluator runs at trace/agent/LLM level and in experiment vs monitor mode.
---

# Add an evaluator (amp-evaluation)

**Read first:** `libs/amp-evaluation/AGENTS.md` → "Defining an evaluator". This skill is the executable checklist. The non-obvious part is that **level and mode are inferred from type hints**, not declared.

## Steps

1. **Pick the file** — a built-in goes in `src/amp_evaluation/evaluators/builtin/` (`standard.py` for rule-based, `llm_judge.py` for judges, `deepeval.py` for DeepEval wrappers). A user-defined one can live anywhere and be picked up by `discover_evaluators(module)`.
2. **Write the function and decorate it:**
   ```python
   from amp_evaluation import evaluator, Trace, Task, EvalResult

   @evaluator("my-check", description="…", tags=["rule-based", "quality"])
   def evaluate(trace: Trace) -> EvalResult:
       ...
   ```
3. **Choose the level via the first parameter's type hint:** `Trace` → TRACE, `AgentTrace` → AGENT, `LLMSpan` → LLM.
4. **Choose the mode via the `task` parameter:**
   - required `task: Task` → **EXPERIMENT only**;
   - `task: Optional[Task] = None` → **both** experiment and monitor;
   - no `task` param → both.
5. **LLM-as-judge:** implement `build_prompt()` (not `evaluate()`) with the same level/mode detection; tag `["llm-judge", <aspect>]`. Needs LLM config via the `any-llm` extra.
6. **Expose config knobs** with the `Param` descriptor: `max_latency_ms: float = Param(default=5000, description="…")`.
7. **Name must be unique** — collisions are rejected in `runner.run()`.

## Gotchas

- Type-hint detection uses `typing.get_type_hints()` — keep annotations importable (avoid forward refs that can't resolve).
- `semantic_similarity`-style judges need `expected_output` on the task, so they're EXPERIMENT-only by nature.
- Return an `EvalResult`; aggregations (mean/stddev) are computed per-evaluator by the runner from its scores.

## Commands (from `libs/amp-evaluation/`)

```bash
pip install -e '.[dev]'
pytest                          # runs with coverage (see pyproject)
ruff check src/                 # lint (line-length 120)
black src/                      # format (project configures Black; don't also run `ruff format`)
mypy src/                       # type-check
```

## Done checklist

- [ ] Correct level from the first-arg type hint; correct mode from the `task` param.
- [ ] Unique name; `tags` set (rule-based / llm-judge + aspect).
- [ ] Test added under `tests/`; `pytest` passes.
- [ ] `ruff check` + `mypy` clean.
