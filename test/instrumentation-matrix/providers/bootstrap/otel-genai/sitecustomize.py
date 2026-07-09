"""Test-only sitecustomize for the otel-genai provider.

Wires a stdlib OpenTelemetry SDK with InMemorySpanExporter so the cell harness
can read captured spans synchronously — identical to the manual provider's
bootstrap. The otel-genai cell constructs OTel-GenAI-native spans directly (no
LLM/network call), so there is no OTLP delivery path to exercise here; the
matrix validates the emitted span *shape* against AMP's contract.
"""
import logging

logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

try:
    import builtins

    from opentelemetry import trace as otel_trace
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import SimpleSpanProcessor
    from opentelemetry.sdk.trace.export.in_memory_span_exporter import (
        InMemorySpanExporter,
    )

    _exporter = InMemorySpanExporter()
    _provider = TracerProvider()
    _provider.add_span_processor(SimpleSpanProcessor(_exporter))
    otel_trace.set_tracer_provider(_provider)

    builtins.__amp_matrix_exporter__ = _exporter
    log.info("matrix-test sitecustomize (otel-genai) initialized")

except Exception as e:  # pragma: no cover
    log.exception("matrix-test sitecustomize (otel-genai) failed: %s", e)
