"""nox driver — one session per emission cell.

Reads matrix.yaml, expands to cells, and runs each cell in its own venv with
the right pinned packages installed. Reports land in reports/cells/.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

import nox

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from harness.manifest import expand_matrix, load_manifest  # noqa: E402
from providers import PROVIDERS  # noqa: E402


def _cells():
    manifest = load_manifest(HERE / "matrix.yaml")
    return expand_matrix(manifest)


CELLS = _cells()


@nox.session(python=False)
@nox.parametrize("cell", CELLS, ids=[c.id for c in CELLS])
def emission(session, cell):
    """Run one emission-tier cell."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--cell-id", default=None)
    args, _ = parser.parse_known_args(session.posargs)
    if args.cell_id and cell.id != args.cell_id:
        session.skip(f"filtered out by --cell-id={args.cell_id}")

    provider = PROVIDERS[cell.provider_name]
    venv_dir = HERE / ".nox" / cell.id
    pip = venv_dir / "bin" / "pip"
    py = venv_dir / "bin" / "python"

    if not py.exists():
        session.run(f"python{cell.python}", "-m", "venv", str(venv_dir), external=True)

    install_specs = [
        *provider.package_specs(cell.provider_version),
        f"{cell.framework_package}=={cell.framework_version}",
        # Per-framework runtime extras declared in matrix.yaml.
        *cell.extras,
        # Test infra.
        "pytest",
        "pytest-recording",
        "vcrpy",
        "jsonschema",
        "pyyaml",
    ]
    session.run(str(pip), "install", "--quiet", *install_specs, external=True)

    pythonpath = f"{HERE}:{provider.bootstrap_module().parent}"

    cell_manifest = {
        "id": cell.id,
        "framework_name": cell.framework_name,
        "framework_package": cell.framework_package,
        "framework_version": cell.framework_version,
        "sample_path": cell.sample_path,
        "span_kinds": cell.span_kinds,
        "contract_schema_id": provider.contract_schema_id(),
    }

    # Pass through real API keys when recording (VCR_RECORD_MODE != "none");
    # otherwise inject dummy values so SDK client constructors don't reject at
    # import time. VCR replays the HTTP before the key is ever used.
    record_mode = os.environ.get("VCR_RECORD_MODE", "none")
    openai_key = os.environ.get("OPENAI_API_KEY") or "test-key-not-used-vcr-replays"
    anthropic_key = (
        os.environ.get("ANTHROPIC_API_KEY") or "test-key-not-used-vcr-replays"
    )

    session.run(
        str(py),
        "-m",
        "pytest",
        "harness/test_cell.py",
        "-v",
        external=True,
        env={
            "PYTHONPATH": pythonpath,
            "VCR_RECORD_MODE": record_mode,
            "CELL_MANIFEST": json.dumps(cell_manifest),
            "REPORTS_DIR": str(HERE / "reports" / "cells"),
            "OPENAI_API_KEY": openai_key,
            "ANTHROPIC_API_KEY": anthropic_key,
        },
    )


@nox.session(python=False)
def report(session):
    """Aggregate per-cell reports into a summary + triage page set."""
    from harness.aggregator import build_summary
    from harness.manifest import load_manifest
    from harness.triage import build_diff_markdown

    reports = HERE / "reports"
    cells_dir = reports / "cells"
    diffs_dir = reports / "diffs"
    diffs_dir.mkdir(parents=True, exist_ok=True)

    if not cells_dir.exists():
        session.error(
            f"no per-cell reports under {cells_dir}; run `nox -s emission` first"
        )

    m = load_manifest(HERE / "matrix.yaml")
    default_id = (
        f"{m.default_cell.provider}-{m.default_cell.provider_version}-"
        f"{m.default_cell.framework}-{m.default_cell.framework_version}-"
        f"py{m.default_cell.python}"
    )

    summary = build_summary(cells_dir, default_cell_id=default_id)
    (reports / "summary.md").write_text(summary)

    for f in cells_dir.glob("*.json"):
        r = json.loads(f.read_text())
        if r["result"] != "fail":
            continue
        # Generic required-set covers the high-signal LLM keys most cells
        # carry; richer per-kind diffs are a follow-up.
        diff = build_diff_markdown(
            r,
            schema_required=[
                "gen_ai.system",
                "gen_ai.request.model",
                "traceloop.span.kind",
            ],
        )
        (diffs_dir / f"{r['cellId']}.diff.md").write_text(diff)

    session.log(f"summary written to {reports / 'summary.md'}")
