from harness.provider import InstrumentationProvider
from providers.manual import ManualProvider
from providers.otel_genai import OtelGenAIProvider
from providers.traceloop import TraceloopProvider

PROVIDERS: dict[str, InstrumentationProvider] = {
    "traceloop": TraceloopProvider(),
    "manual": ManualProvider(),
    "otel-genai": OtelGenAIProvider(),
}
