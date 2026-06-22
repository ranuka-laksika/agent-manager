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

// Validates the full operator-to-developer flow:
//  1. Create a new environment by executing the add-environment script
//     (which provisions the API Platform Gateway via helm).
//  2. Create a deployment pipeline spanning the default and new environments.
//  3. Create a project that uses the new pipeline.
//  4. Create an internal agent with source code; build and deploy it to default.
//  5. Promote the agent to the new environment with new env values, then verify
//     it deploys and serves traffic there.

package environment

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	dpops "github.com/wso2/agent-manager/test/e2e/operations/deploymentpipeline"
	envops "github.com/wso2/agent-manager/test/e2e/operations/environment"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
)

var _ = Describe("Environment Provisioning and Promotion Lifecycle",
	Label("environment", "promotion"), Ordered, func() {
		var (
			envName      string
			pipelineName string
			projectName  string
			agentName    string
			endpointURL  string
			apiKey       string
			invokeReq    json.RawMessage

			scriptParams *envops.ScriptParams
		)

		BeforeAll(func() {
			Expect(Cfg.TavilyAPIKey).NotTo(BeEmpty(), "TAVILY_API_KEY must be set")
			Expect(Cfg.OpenAIAPIKey).NotTo(BeEmpty(), "OPENAI_API_KEY must be set")

			suffix := uuid.New().String()[:6]
			// All names use the e2e prefixes so the centralized stale-resource
			// sweep (testsetup.CleanupStaleE2EResources, run in the root suite's
			// BeforeSuite) reaps them on a later run. Environment names are
			// length-constrained by the gateway Service name (k8s 63-char limit),
			// so the env prefix is kept short.
			envName = framework.E2EEnvPrefix + suffix
			projectName = framework.E2EProjectPrefix + "promo-" + suffix
			agentName = "e2e-promo-agent-" + suffix

			scriptParams = envops.FromClient(Client)
			scriptParams.EnvName = envName
			scriptParams.DisplayName = "E2E Promo " + suffix
		})

		It("should create the environment via the add-environment script", func() {
			envops.AddEnvironment(scriptParams)

			env := envops.GetEnvironment(Default, Client, Cfg.DefaultOrg, envName)
			Expect(env.Name).To(Equal(envName))
			GinkgoWriter.Printf("Environment created: %s\n", envName)
		})

		It("should create a deployment pipeline spanning default and the new environment", func() {
			req := framework.CreateDeploymentPipelineRequest{
				DisplayName: "E2E Promo Pipeline " + envName,
				PromotionPaths: []framework.PromotionPath{
					{
						SourceEnvironmentRef: Cfg.DefaultEnv,
						TargetEnvironmentRefs: []framework.TargetEnvironmentRef{
							{Name: envName},
						},
					},
				},
			}
			pipeline := dpops.Create(Default, Client, Cfg.DefaultOrg, req)
			pipelineName = pipeline.Name
			Expect(pipelineName).NotTo(BeEmpty(), "created pipeline should have a name")
			GinkgoWriter.Printf("Deployment pipeline created: %s\n", pipelineName)
		})

		It("should create a project that uses the new pipeline", func() {
			req := framework.NewCreateProjectRequest(projectName, "E2E Promo Project", "Project for promotion lifecycle e2e")
			req.DeploymentPipeline = pipelineName

			proj := project.CreateProject(Default, Client, &project.CreateProjectParams{
				OrgName: Cfg.DefaultOrg,
				Request: req,
			})
			Expect(proj.Name).To(Equal(projectName))
			Expect(proj.DeploymentPipeline).To(Equal(pipelineName))
			GinkgoWriter.Printf("Project created: %s (pipeline=%s)\n", projectName, pipelineName)
		})

		It("should create an internal agent with source code", func() {
			envVars := map[string]string{
				"TAVILY_API_KEY": Cfg.TavilyAPIKey,
				"OPENAI_API_KEY": Cfg.OpenAIAPIKey,
				"DATABASE_URL":   "http://localhost:5000",
			}
			createReq := framework.NewInternalChatAgentRequest(agentName, "Internal chat agent for promotion lifecycle test", envVars)

			ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
				OrgName:     Cfg.DefaultOrg,
				ProjectName: projectName,
				Request:     createReq,
			})
			Expect(ag.Name).To(Equal(agentName))
			GinkgoWriter.Printf("Agent created: %s\n", agentName)
		})

		It("should complete the build", func() {
			buildName := build.WaitForBuildSuccess(Client, &build.WaitForBuildParams{
				OrgName:     Cfg.DefaultOrg,
				ProjectName: projectName,
				AgentName:   agentName,
				Timeout:     20 * time.Minute,
			})
			GinkgoWriter.Printf("Build completed: %s\n", buildName)
		})

		It("should deploy to the default environment", func() {
			deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
				OrgName:     Cfg.DefaultOrg,
				ProjectName: projectName,
				AgentName:   agentName,
				Environment: Cfg.DefaultEnv,
				Timeout:     5 * time.Minute,
			})

			agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
				OrgName:     Cfg.DefaultOrg,
				ProjectName: projectName,
				AgentName:   agentName,
				Environment: Cfg.DefaultEnv,
				SearchText:  "Uvicorn running on",
				Timeout:     10 * time.Minute,
			})
			GinkgoWriter.Printf("Agent deployed and ready in %s\n", Cfg.DefaultEnv)
		})

		It("should promote to the new environment with new env values", func() {
			useSource := false
			promoteReq := framework.PromoteAgentRequest{
				SourceEnvironment:      Cfg.DefaultEnv,
				TargetEnvironment:      envName,
				UseConfigFromSourceEnv: &useSource,
				Env: []framework.EnvironmentVariable{
					{Key: "TAVILY_API_KEY", Value: Cfg.TavilyAPIKey, IsSensitive: true},
					{Key: "OPENAI_API_KEY", Value: Cfg.OpenAIAPIKey, IsSensitive: true},
					// New value specific to the target environment, distinct from the
					// source env's DATABASE_URL, to prove per-env overrides take effect.
					{Key: "DATABASE_URL", Value: "http://localhost:6000"},
					{Key: "DEPLOY_ENV", Value: envName},
				},
			}
			resp := agentops.PromoteAgent(Default, Client, Cfg.DefaultOrg, projectName, agentName, promoteReq)
			Expect(resp.TargetEnvironment).To(Equal(envName))
			GinkgoWriter.Printf("Agent promoted: %s -> %s\n", Cfg.DefaultEnv, envName)
		})

		It("should deploy to the new environment", func() {
			deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
				OrgName:     Cfg.DefaultOrg,
				ProjectName: projectName,
				AgentName:   agentName,
				Environment: envName,
				Timeout:     5 * time.Minute,
			})

			agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
				OrgName:     Cfg.DefaultOrg,
				ProjectName: projectName,
				AgentName:   agentName,
				Environment: envName,
				SearchText:  "Uvicorn running on",
				Timeout:     10 * time.Minute,
			})
			GinkgoWriter.Printf("Agent deployed and ready in %s\n", envName)
		})

		It("should serve traffic in the new environment", func() {
			endpoints := deployment.GetEndpoints(Default, Client,
				Cfg.DefaultOrg, projectName, agentName, envName)
			for _, ep := range endpoints {
				if ep.URL != "" {
					endpointURL = ep.URL
					break
				}
			}
			Expect(endpointURL).NotTo(BeEmpty(), "agent endpoint URL in new env should not be empty")

			apiKeyResp := agentops.CreateAgentAPIKey(Default, Client,
				Cfg.DefaultOrg, projectName, agentName, envName,
				framework.CreateAgentAPIKeyRequest{
					DisplayName: "e2e-promo-key",
					ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				})
			apiKey = apiKeyResp.ApiKey
			Expect(apiKey).NotTo(BeEmpty(), "agent API key in new env should not be empty")

			invokeReq = framework.DefaultInvokeRequest()
			endpoint := fmt.Sprintf("%s/chat", endpointURL)
			GinkgoWriter.Printf("Invoking promoted agent at: %s\n", endpoint)
			agentops.InvokeAgentEndpoint(endpoint, invokeReq, apiKey)
		})
	})
