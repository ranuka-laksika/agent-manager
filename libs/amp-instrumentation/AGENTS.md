# amp-instrumentation — agent guide

Zero-code OpenTelemetry instrumentation for Python AI agents, built on the **Traceloop SDK**. Ships two activation paths (auto-inject CLI + manual helper) and exports OTLP/HTTP traces to the platform. Packaged with **hatchling**; Python ≥ 3.10.

## Layout

`src/amp_instrumentation/`
| File | Role |
|---|---|
| `__init__.py` | public API — exports `init_otel()` |
| `otel.py` | **manual** helper: builds a `TracerProvider` + `BatchSpanProcessor`, exports to `AMP_OTEL_ENDPOINT/v1/traces` with the `x-amp-api-key` header |
| `cli/main.py` | `amp-instrument` CLI — prepends `_bootstrap/` to `PYTHONPATH` and execs the target |
| `_bootstrap/sitecustomize.py` | auto-imported by Python at startup → `initialize_instrumentation()` |
| `_bootstrap/initialization.py` | validates env vars, sets Traceloop config, calls `Traceloop.init()` |
| `_bootstrap/constants.py` | all `AMP_*` env-var names |

## The two activation paths

1. **Auto (CLI)** — `amp-instrument python app.py`. The CLI puts `_bootstrap/` first on `PYTHONPATH`; Python auto-imports `sitecustomize.py`, which runs `Traceloop.init()` with the OTLP endpoint + API key. No code change in the target app.
2. **Manual** — call `init_otel()` in code. Configures the OTel tracer provider directly. Idempotent via a global flag + `threading.Lock()`.

Framework coverage is whatever **Traceloop** instruments (LangChain, OpenAI, Anthropic, …) — this package does not hand-write per-framework instrumentors.

## Configuration (env vars, from `_bootstrap/constants.py`)

| Var | Purpose |
|---|---|
| `AMP_OTEL_ENDPOINT` | OTLP/HTTP collector base URL (required) |
| `AMP_AGENT_API_KEY` | agent API key → sent as `x-amp-api-key` (required) |
| `AMP_TRACE_CONTENT` | capture prompt/completion content |
| `AMP_DEBUG` | `1` → debug logging to stderr |

Missing required vars raise a `ConfigurationError` at init.

## Commands

No Makefile in the package dir. From here:
```bash
pip install -e .          # local install
pytest                    # run tests (config in pytest.ini, tests in tests/)
amp-instrument python x.py  # exercise the CLI path
```
Tests use **pytest** with `conftest.py` fixtures (`clean_environment`, `mock_traceloop`). No ruff/black/mypy config in this package — follow the repo-root conventions.

## Pinned-version gotchas

- **`traceloop-sdk==0.61.0`** is pinned exactly — bumping it is a deliberate release, not a casual change.
- **`wrapt<2.0.0`** is required: wrapt 2.x dropped the `module=` kwarg still used by `opentelemetry-instrumentation-*` 0.61.0.
- A `sitecustomize.py` in the app's own working dir will shadow the bootstrap one (a warning is printed) — don't ship one in instrumented apps.
- `init_otel()` reads `AMP_OTEL_ENDPOINT` / `AMP_AGENT_API_KEY` at call time, not import time.

See also the sibling init-container that ships this package into pods: `../../python-instrumentation-provider/AGENTS.md`.
