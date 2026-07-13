// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

using System.ClientModel;
using System.ComponentModel;
using Microsoft.Agents.AI;
using Microsoft.Extensions.AI;
using OpenAI;
using OpenAI.Chat;

namespace AmpDotnetAgent;

/// <summary>
/// Builds the sample agent on Microsoft Agent Framework and turns on its
/// OpenTelemetry emission. The single <c>UseOpenTelemetry</c> call below is the
/// only thing needed to produce AMP-compatible <c>gen_ai.*</c> spans — Agent
/// Framework follows the OpenTelemetry GenAI semantic conventions natively, so
/// one <c>/chat</c> call yields:
///
///   invoke_agent WeatherAgent   (root, from the agent)
///   └── chat &lt;model&gt;             (the LLM call, auto-wired below)
///       └── execute_tool GetWeather  (when the model calls the tool)
///
/// which is the same span shape AMP's Python agents emit.
/// </summary>
public static class AgentFactory
{
    private static readonly string Model =
        Environment.GetEnvironmentVariable("OPENAI_MODEL") ?? "gpt-4o-mini";

    public static AIAgent Create()
    {
        var apiKey = Environment.GetEnvironmentVariable("OPENAI_API_KEY")
            ?? throw new InvalidOperationException(
                "OPENAI_API_KEY is required but not set.");

        // Optional endpoint override for OpenAI-compatible backends (Azure OpenAI,
        // a gateway, or a local server). Leave OPENAI_BASE_URL unset for OpenAI.
        var baseUrl = Environment.GetEnvironmentVariable("OPENAI_BASE_URL");
        var clientOptions = string.IsNullOrWhiteSpace(baseUrl)
            ? null
            : new OpenAIClientOptions { Endpoint = new Uri(baseUrl) };

        // OpenAI SDK chat client -> Microsoft.Extensions.AI IChatClient, passed to
        // the agent uninstrumented. UseOpenTelemetry on the *agent* auto-wires the
        // chat-client instrumentation below the framework's function-invoking layer,
        // which is what makes the spans nest correctly (invoke_agent -> chat ->
        // execute_tool), all under AmpTelemetry.SourceName. Instrumenting the chat
        // client yourself as well would disable that auto-wiring and flatten the
        // trace.
        IChatClient chatClient =
            new ChatClient(Model, new ApiKeyCredential(apiKey), clientOptions)
                .AsIChatClient();

        return new ChatClientAgent(
                chatClient,
                name: "WeatherAgent",
                instructions:
                    "You are a helpful weather assistant. Use the tools available "
                    + "to answer questions about the weather. Keep answers concise.",
                tools: [AIFunctionFactory.Create(GetWeather)])
            .AsBuilder()
            .UseOpenTelemetry(
                sourceName: AmpTelemetry.SourceName,
                configure: cfg => cfg.EnableSensitiveData = AmpTelemetry.TraceContentEnabled)
            .Build();
    }

    [Description("Get the current weather for a given city.")]
    private static string GetWeather(
        [Description("The city to get the weather for.")] string city)
    {
        // A real agent would call a weather API here. Kept deterministic so the
        // sample runs with only an OpenAI key and produces a stable trace.
        return $"The weather in {city} is sunny with a high of 24°C.";
    }
}
