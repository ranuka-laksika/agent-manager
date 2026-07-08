"""OTel-GenAI-native cell sample.

Re-expresses, in Python, the span shape Microsoft Agent Framework (.NET) emits —
the same shape ``samples/dotnet-agent/`` produces — so the OTel-native contract
is locked into the emission matrix and re-checked on every PR that touches the
observer. There is no LLM/network call: the spans are constructed directly from
a fixed, representative interaction (the weather-agent tool call documented in
``samples/dotnet-agent/README.md``), which keeps the cell deterministic and
cassette-free.

Trace shape (matches samples/dotnet-agent):

    invoke_agent WeatherAgent
    ├── chat {model}                 (model decides to call the tool)
    │   └── execute_tool GetWeather
    └── chat {model}                 (final answer)

Distinctive OTel-native traits exercised here (vs the Traceloop shape):
  * span names "{operation} {model}" (no vendor prefix, no ".chat" suffix)
  * gen_ai.provider.name instead of gen_ai.system
  * structured parts[] messages instead of flat content strings
  * gen_ai.tool.call.arguments / gen_ai.tool.call.result for tool I/O
"""
from __future__ import annotations

import json

from opentelemetry import trace

_MODEL = "gpt-4o-mini"
_PROVIDER = "openai"

# Structured "parts[]" messages — the current OTel GenAI shape (not flat content).
_USER_MSG = json.dumps(
    [{"role": "user", "parts": [{"type": "text", "content": "What is the weather in Tokyo?"}]}]
)
_FINAL_MSG = json.dumps(
    [{"role": "assistant", "parts": [{"type": "text", "content": "It is sunny in Tokyo, 24C."}]}]
)


def run_scenario() -> None:
    tracer = trace.get_tracer("otel-native-agent-sample")

    with tracer.start_as_current_span("invoke_agent WeatherAgent") as agent_span:
        agent_span.set_attribute("gen_ai.operation.name", "invoke_agent")
        agent_span.set_attribute("gen_ai.provider.name", _PROVIDER)
        agent_span.set_attribute("gen_ai.agent.name", "WeatherAgent")
        agent_span.set_attribute("gen_ai.request.model", _MODEL)
        agent_span.set_attribute("gen_ai.input.messages", _USER_MSG)
        agent_span.set_attribute("gen_ai.output.messages", _FINAL_MSG)
        agent_span.set_attribute("gen_ai.usage.input_tokens", 40)
        agent_span.set_attribute("gen_ai.usage.output_tokens", 20)

        # First chat: the model requests the tool.
        with tracer.start_as_current_span(f"chat {_MODEL}") as chat1:
            _set_chat_attrs(chat1, input_tokens=20, output_tokens=10)
            chat1.set_attribute("gen_ai.input.messages", _USER_MSG)
            chat1.set_attribute("gen_ai.response.finish_reasons", json.dumps(["tool_calls"]))

            with tracer.start_as_current_span("execute_tool GetWeather") as tool_span:
                tool_span.set_attribute("gen_ai.operation.name", "execute_tool")
                tool_span.set_attribute("gen_ai.tool.name", "GetWeather")
                tool_span.set_attribute(
                    "gen_ai.tool.call.arguments", json.dumps({"city": "Tokyo"})
                )
                tool_span.set_attribute(
                    "gen_ai.tool.call.result", "The weather in Tokyo is sunny, 24C."
                )

        # Second chat: the model produces the final answer.
        with tracer.start_as_current_span(f"chat {_MODEL}") as chat2:
            _set_chat_attrs(chat2, input_tokens=20, output_tokens=10)
            chat2.set_attribute("gen_ai.output.messages", _FINAL_MSG)
            chat2.set_attribute("gen_ai.response.finish_reasons", json.dumps(["stop"]))


def _set_chat_attrs(span, *, input_tokens: int, output_tokens: int) -> None:
    span.set_attribute("gen_ai.operation.name", "chat")
    span.set_attribute("gen_ai.provider.name", _PROVIDER)
    span.set_attribute("gen_ai.request.model", _MODEL)
    span.set_attribute("gen_ai.response.model", _MODEL)
    span.set_attribute("gen_ai.usage.input_tokens", input_tokens)
    span.set_attribute("gen_ai.usage.output_tokens", output_tokens)
