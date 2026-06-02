from harness.categorize import FailureCategory
from heavy import diagnostics
from heavy.amp_client import DeployedAgent
from heavy.diagnostics import Evidence, classify_no_spans


def _agent(name="traceloop-0-61-0-l-1476e2"):
    return DeployedAgent(org="default", project_name="p", agent_name=name,
                         environment="dev", endpoint_url="http://x", api_key="k")


def test_boundary_categories_exist():
    assert FailureCategory.INGEST_REJECTED.value == "ingest-rejected"
    assert FailureCategory.EXPORT_FAILED.value == "export-failed"
    assert FailureCategory.COLLECTOR_NOT_RECEIVED.value == "collector-not-received"


def test_classify_ingest_rejected_on_401():
    ev = Evidence(agent_init="ok", agent_export_status=401, agent_export_error="401 Unauthorized")
    assert classify_no_spans(ev) is FailureCategory.INGEST_REJECTED


def test_classify_ingest_rejected_on_403():
    ev = Evidence(agent_init="ok", agent_export_status=403)
    assert classify_no_spans(ev) is FailureCategory.INGEST_REJECTED


def test_classify_export_failed_on_init_failure():
    ev = Evidence(agent_init="failed")
    assert classify_no_spans(ev) is FailureCategory.EXPORT_FAILED


def test_classify_export_failed_on_5xx():
    ev = Evidence(agent_init="ok", agent_export_status=503, agent_export_error="503")
    assert classify_no_spans(ev) is FailureCategory.EXPORT_FAILED


def test_classify_export_failed_on_error_without_status():
    ev = Evidence(agent_init="ok", agent_export_error="connection refused")
    assert classify_no_spans(ev) is FailureCategory.EXPORT_FAILED


def test_classify_collector_not_received_when_agent_ok_but_collector_empty():
    ev = Evidence(agent_init="ok", collector_received=False)
    assert classify_no_spans(ev) is FailureCategory.COLLECTOR_NOT_RECEIVED


def test_classify_falls_back_to_no_spans_when_inconclusive():
    assert classify_no_spans(Evidence()) is FailureCategory.NO_SPANS_CAPTURED


def test_collect_evidence_parses_401_from_agent_log(monkeypatch):
    def fake_kubectl(args):
        if args[:3] == ["get", "pods", "-A"]:
            return 0, "amp-dp traceloop-0-61-0-l-1476e2-abc123\n"
        if args[0] == "logs" and "traceloop-0-61-0-l-1476e2-abc123" in args:
            return 0, ("Automatic Tracing initialized successfully.\n"
                       "Failed to export batch code: 401, reason: Unauthorized\n")
        if args[0] == "get" and "deploy" in args:
            return 0, "observability opentelemetry-collector\n"
        if args[0] == "logs" and "deploy/opentelemetry-collector" in args:
            return 0, "no spans here\n"
        return 1, ""
    monkeypatch.setattr(diagnostics, "_kubectl", fake_kubectl)
    ev = diagnostics.collect_failure_evidence(_agent())
    assert ev.agent_init == "ok"
    assert ev.agent_export_status == 401


def test_collect_evidence_detects_init_failure(monkeypatch):
    def fake_kubectl(args):
        if args[:3] == ["get", "pods", "-A"]:
            return 0, "amp-dp traceloop-0-61-0-l-1476e2-abc123\n"
        if args[0] == "logs":
            return 0, "Failed to initialize Automatic Tracing: boom\n"
        return 1, ""
    monkeypatch.setattr(diagnostics, "_kubectl", fake_kubectl)
    ev = diagnostics.collect_failure_evidence(_agent())
    assert ev.agent_init == "failed"


def test_collect_evidence_never_raises_when_kubectl_unavailable(monkeypatch):
    monkeypatch.setattr(diagnostics, "_kubectl", lambda args: (127, ""))
    ev = diagnostics.collect_failure_evidence(_agent())
    assert ev.agent_init is None and ev.agent_export_status is None
