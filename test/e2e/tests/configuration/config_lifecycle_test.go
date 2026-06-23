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

// Validates configuration updates on a dedicated, disposable single-environment
// IT helpdesk agent: redeployment with modified env vars, verification of
// non-secret config changes, and detection of an invalid API key.

package configuration

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
)

var _ = Describe("Agent runtime configurations:", Label("configuration", "single-env"), Ordered, func() {
	var (
		sensitiveSecretRef string
		lastDeployedBefore time.Time
	)

	// The dedicated agent is single-use; delete it so its workload pod is freed.
	// No config restore is needed since nothing else reuses this agent.
	AfterAll(func() {
		agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, ConfigAgent.ProjectName, ConfigAgent.AgentName)
	})

	It("reports the agent's initial environment configuration", func() {
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, ConfigAgent.ProjectName, ConfigAgent.AgentName, Cfg.DefaultEnv)

		var foundDB, foundOpenAI bool
		for _, c := range configs.Configurations.Env {
			if c.Key == "DATABASE_URL" {
				foundDB = true
				Expect(c.Value).To(Equal("http://localhost:5000"), "DATABASE_URL should have initial value")
				Expect(c.IsSensitive).To(BeFalse(), "DATABASE_URL should not be sensitive")
			}
			if c.Key == "OPENAI_API_KEY" {
				foundOpenAI = true
				Expect(c.IsSensitive).To(BeTrue(), "OPENAI_API_KEY should be sensitive")
				Expect(c.Value).To(BeEmpty(), "sensitive config value should be masked")
				Expect(c.SecretRef).NotTo(BeEmpty(), "OPENAI_API_KEY should have a secretRef")
				sensitiveSecretRef = c.SecretRef
			}
		}
		Expect(foundDB).To(BeTrue(), "DATABASE_URL should exist in the initial configuration")
		Expect(foundOpenAI).To(BeTrue(), "OPENAI_API_KEY should exist in the initial configuration")
		GinkgoWriter.Printf("Initial configurations verified; OPENAI_API_KEY secretRef=%s\n", sensitiveSecretRef)
	})

	It("redeploys the agent with updated environment variables", func() {
		deps := deployment.GetDeploymentDetails(Default, Client,
			Cfg.DefaultOrg, ConfigAgent.ProjectName, ConfigAgent.AgentName)
		dep, exists := deps[Cfg.DefaultEnv]
		Expect(exists).To(BeTrue(), "deployment should exist for default environment")
		Expect(dep.ImageID).NotTo(BeEmpty(), "imageId should not be empty")
		lastDeployedBefore = dep.LastDeployed

		autoInstr := true
		deployment.DeployAgent(Default, Client, Cfg.DefaultOrg, ConfigAgent.ProjectName, ConfigAgent.AgentName,
			framework.DeployAgentRequest{
				ImageID: dep.ImageID,
				Env: []framework.EnvironmentVariable{
					{Key: "OPENAI_API_KEY", Value: "sk-invalid-key-for-e2e-test-" + uuid.New().String()[:8], IsSensitive: true},
					{Key: "DATABASE_URL", Value: "http://localhost:6000", IsSensitive: false},
				},
				EnableAutoInstrumentation: &autoInstr,
			})
		GinkgoWriter.Printf("Redeployment triggered with updated configurations\n")
	})

	It("becomes active after the configuration redeploy", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:       Cfg.DefaultOrg,
			ProjectName:   ConfigAgent.ProjectName,
			AgentName:     ConfigAgent.AgentName,
			Environment:   Cfg.DefaultEnv,
			Timeout:       5 * time.Minute,
			DeployedAfter: lastDeployedBefore,
		})
	})

	It("becomes ready after the configuration redeploy", func() {
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, ConfigAgent.ProjectName, ConfigAgent.AgentName, Cfg.DefaultEnv, 10*time.Minute)
	})

	It("reflects the updated non-secret configuration values", func() {
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, ConfigAgent.ProjectName, ConfigAgent.AgentName, Cfg.DefaultEnv)

		var foundDB bool
		for _, c := range configs.Configurations.Env {
			if c.Key == "DATABASE_URL" {
				foundDB = true
				Expect(c.Value).To(Equal("http://localhost:6000"),
					"DATABASE_URL should have the updated value")
			}
		}
		Expect(foundDB).To(BeTrue(), "DATABASE_URL should still exist after redeploy")
	})

	It("surfaces the invalid-API-key error in the agent runtime logs", func() {
		By("Invoking the agent so the runtime attempts a LLM call")
		// requireOK=false: the invocation is expected to error (invalid key); we only
		// need it to reach the runtime so the failure is logged.
		agentops.InvokeAgentEndpoint(
			ConfigAgent.EndpointURL+"/chat",
			ConfigAgent.InvokeReq,
			ConfigAgent.APIKey,
			false)

		By("Waiting for the invalid-API-key error in the runtime logs")
		agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: ConfigAgent.ProjectName,
			AgentName:   ConfigAgent.AgentName,
			Environment: Cfg.DefaultEnv,
			SearchText:  "Incorrect API key provided",
			Timeout:     10 * time.Minute,
		})
	})
})
