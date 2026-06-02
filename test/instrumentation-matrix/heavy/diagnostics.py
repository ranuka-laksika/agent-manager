"""Heavy-tier failure diagnostics: capture pipeline-boundary evidence when a
cell captures 0 spans, and classify which boundary the trail went cold at.

Evidence is gathered best-effort from `kubectl logs` (agent pod + otel
collector); any kubectl failure leaves the corresponding field None
(indeterminate) and never raises. The classifier is pure so it unit-tests
without a cluster.
"""
from __future__ import annotations

import re
import subprocess
from dataclasses import dataclass, field

from harness.categorize import FailureCategory
from heavy.amp_client import DeployedAgent

_COLLECTOR_DEPLOY = "opentelemetry-collector"
_LOG_TAIL = "300"


@dataclass
class Evidence:
    agent_init: str | None = None          # "ok" | "failed" | None (marker not found)
    agent_export_status: int | None = None # HTTP status parsed from an OTLP exporter error line
    agent_export_error: str | None = None  # short excerpt of the exporter error
    collector_received: bool | None = None # True / False / None (indeterminate)
    raw: dict[str, str] = field(default_factory=dict)  # truncated log excerpts, for the report


def classify_no_spans(ev: Evidence) -> FailureCategory:
    """Map boundary evidence to a failure category. Order matters: a gateway
    rejection (401/403) is the most specific and actionable signal."""
    if ev.agent_export_status in (401, 403):
        return FailureCategory.INGEST_REJECTED
    if (
        ev.agent_init == "failed"
        or (ev.agent_export_status is not None and ev.agent_export_status >= 500)
        or (ev.agent_export_error is not None and ev.agent_export_status is None)
    ):
        return FailureCategory.EXPORT_FAILED
    if ev.agent_init == "ok" and ev.agent_export_error is None and ev.collector_received is False:
        return FailureCategory.COLLECTOR_NOT_RECEIVED
    return FailureCategory.NO_SPANS_CAPTURED


def _kubectl(args: list[str]) -> tuple[int, str]:
    """Run a kubectl command best-effort; return (returncode, stdout+stderr)."""
    try:
        proc = subprocess.run(
            ["kubectl", *args], capture_output=True, text=True, timeout=30
        )
        return proc.returncode, (proc.stdout or "") + (proc.stderr or "")
    except Exception:  # noqa: BLE001 - diagnostics must never raise
        return 127, ""


def _find_pod(agent_name: str) -> tuple[str, str] | None:
    """Return (namespace, pod) for the deployed agent's pod, or None."""
    rc, out = _kubectl([
        "get", "pods", "-A", "--no-headers",
        "-o", "custom-columns=NS:.metadata.namespace,NAME:.metadata.name",
    ])
    if rc != 0:
        return None
    for line in out.splitlines():
        parts = line.split()
        if len(parts) == 2 and agent_name in parts[1]:
            return parts[0], parts[1]
    return None


def _agent_signals(agent_name: str, ev: Evidence) -> None:
    pod = _find_pod(agent_name)
    if pod is None:
        return
    ns, name = pod
    rc, log = _kubectl(["logs", "-n", ns, name, "--all-containers", "--tail", _LOG_TAIL])
    if rc != 0 or not log:
        return
    ev.raw["agent"] = log[-2000:]
    if "Automatic Tracing initialized successfully" in log:
        ev.agent_init = "ok"
    if "Failed to initialize Automatic Tracing" in log:
        ev.agent_init = "failed"
    for line in log.splitlines():
        if "failed to export" in line.lower():
            ev.agent_export_error = line.strip()[:300]
            m = re.search(r"\b(\d{3})\b", line)
            if m:
                ev.agent_export_status = int(m.group(1))
            break


def _collector_signals(ev: Evidence) -> None:
    rc, out = _kubectl([
        "get", "deploy", "-A", "--no-headers",
        "-o", "custom-columns=NS:.metadata.namespace,NAME:.metadata.name",
    ])
    if rc != 0:
        return
    ns = None
    for line in out.splitlines():
        parts = line.split()
        if len(parts) == 2 and _COLLECTOR_DEPLOY in parts[1]:
            ns = parts[0]
            break
    if ns is None:
        return
    rc, log = _kubectl(["logs", "-n", ns, f"deploy/{_COLLECTOR_DEPLOY}", "--tail", _LOG_TAIL])
    if rc != 0:
        return
    ev.raw["collector"] = log[-2000:]
    # The collector logs span receipt; absence of any receipt marker means it
    # never got the agent's spans. Keep the marker list narrow and reviewable.
    ev.collector_received = ("TracesExporter" in log) or ("spans" in log.lower())


def collect_failure_evidence(deployed: DeployedAgent) -> Evidence:
    """Gather agent + collector log signals for a 0-span cell. Best-effort:
    each probe is independent and any failure leaves its fields None."""
    ev = Evidence()
    _agent_signals(deployed.agent_name, ev)
    _collector_signals(ev)
    return ev
