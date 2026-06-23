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

// Validates LLM provider integration with the IT helpdesk agent: provider creation,
// agent deployment with USE_LLM_PROVIDER=true, and LLM env var injection.

package llmprovider

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
)

var _ = Describe("LLM provider configured for an internal agent:", Label("llm-provider", "internal-agent"), Ordered, func() {
	var (
		agentName string
		suffix    string

		providerID  string
		gatewayUUID string

		endpointURL string
		invokeReq   json.RawMessage

		createReq framework.CreateAgentRequest
	)

	BeforeAll(func() {
		suffix = uuid.New().String()[:8]
		agentName = framework.E2EAgentPrefix + suffix
		providerID = framework.E2ELLMProviderPrefix + suffix

		createReq = framework.NewITHelpdeskAgentRequest(
			agentName,
			"IT helpdesk agent for e2e LLM provider config test",
			map[string]string{"USE_LLM_PROVIDER": "true"},
		)
	})

	It("finds an active AI gateway for the default environment", func() {
		gatewayUUID = gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
	})

	It("creates an OpenAI-backed LLM provider from the built-in template", func() {
		By("Fetching the OpenAI template to get endpoint URL and auth config")
		templates := llmproviderop.ListLLMProviderTemplates(Default, Client, Cfg.DefaultOrg)
		var openaiTpl *framework.LLMProviderTemplateResponse
		for i, t := range templates.Templates {
			if t.ID == "openai" {
				openaiTpl = &templates.Templates[i]
				break
			}
		}
		Expect(openaiTpl).NotTo(BeNil(), "expected built-in 'openai' template to exist")
		Expect(openaiTpl.Metadata).NotTo(BeNil(), "expected template metadata")
		Expect(openaiTpl.Metadata.EndpointURL).NotTo(BeEmpty(), "expected template endpoint URL")
		Expect(openaiTpl.Metadata.Auth).NotTo(BeNil(), "expected template auth config")
		GinkgoWriter.Printf("OpenAI template: url=%s, auth.type=%s, auth.header=%s\n",
			openaiTpl.Metadata.EndpointURL, openaiTpl.Metadata.Auth.Type, openaiTpl.Metadata.Auth.Header)

		By("Creating the LLM provider")
		upstreamURL := openaiTpl.Metadata.EndpointURL
		authHeader := openaiTpl.Metadata.Auth.Header
		authValue := openaiTpl.Metadata.Auth.ValuePrefix + Cfg.OpenAIAPIKey

		prov := llmproviderop.CreateLLMProvider(Default, Client, Cfg.DefaultOrg,
			framework.CreateLLMProviderRequest{
				ID:       providerID,
				Name:     "E2E Internal OpenAI Provider " + suffix,
				Version:  "v1.0",
				Context:  "/" + providerID,
				Template: "openai",
				Upstream: framework.UpstreamConfig{
					Main: &framework.UpstreamEndpoint{
						URL: &upstreamURL,
						Auth: &framework.UpstreamAuth{
							Type:   openaiTpl.Metadata.Auth.Type,
							Header: &authHeader,
							Value:  &authValue,
						},
					},
				},
				Gateways: []string{gatewayUUID},
			})
		Expect(prov.UUID).NotTo(BeEmpty())
		GinkgoWriter.Printf("LLM provider: %s (UUID: %s)\n", providerID, prov.UUID)
	})

	It("creates an internal agent wired to the LLM provider via model config", func() {
		createReq.ModelConfig = []framework.ModelConfigRequest{
			{
				ProviderName: providerID,
				EnvironmentVariables: []framework.EnvironmentVariableConfig{
					{Key: "url", Name: "LLM_PROVIDER_URL"},
					{Key: "apikey", Name: "LLM_PROVIDER_KEY"},
				},
			},
		}

		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		GinkgoWriter.Printf("Agent: %s (with modelConfig)\n", agentName)
	})

	It("builds the agent image to completion", func() {
		build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Timeout:     20 * time.Minute,
		})
	})

	It("deploys the agent to the default environment", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			AgentName:   agentName,
			Environment: Cfg.DefaultEnv,
			Timeout:     5 * time.Minute,
		})
	})

	It("becomes ready and exposes an invocation endpoint", func() {
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, 10*time.Minute)

		endpoints := deployment.GetEndpoints(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)
		for _, ep := range endpoints {
			if ep.URL != "" {
				endpointURL = ep.URL
				break
			}
		}
		Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL should not be empty")

		invokeReq = framework.DefaultInvokeRequest()
		GinkgoWriter.Printf("Agent ready: endpoint=%s\n", endpointURL)
	})

	It("returns a chat response routed through the LLM provider", func() {
		// Create the API key in the same It as the invocation so its DeferCleanup
		// revoke runs only after the invocation completes (keeps key count bounded).
		apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "e2e-llm-provider-key",
				ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			})
		Expect(apiKeyResp.ApiKey).NotTo(BeEmpty(), "agent API key should not be empty")

		endpoint := endpointURL + "/chat"
		GinkgoWriter.Printf("Endpoint: %s\n", endpoint)
		agentops.InvokeAgentEndpoint(endpoint, invokeReq, apiKeyResp.ApiKey)
	})

	It("verify the injected LLM provider URL and key into the agent configuration", func() {
		By("Verifying configurations include LLM provider variables")
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv)

		GinkgoWriter.Printf("Agent configurations (%d items):\n", len(configs.Configurations.Env))
		for _, c := range configs.Configurations.Env {
			GinkgoWriter.Printf("  %s (sensitive: %v)\n", c.Key, c.IsSensitive)
		}

		configKeys := make(map[string]bool)
		for _, c := range configs.Configurations.Env {
			configKeys[c.Key] = true
		}

		Expect(configKeys).To(HaveKey("LLM_PROVIDER_URL"),
			"LLM_PROVIDER_URL from model config should be in configurations")
		Expect(configKeys).To(HaveKey("LLM_PROVIDER_KEY"),
			"LLM_PROVIDER_KEY from model config should be in configurations")
	})
})
