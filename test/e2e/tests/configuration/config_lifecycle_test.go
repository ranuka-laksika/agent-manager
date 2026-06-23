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

// Validates configuration updates on the shared single-environment IT helpdesk
// agent: redeployment with modified env vars, verification of
// non-secret config changes, and detection of an invalid API key.

package configuration

import (
	"time"

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

	// These tests mutate the shared agent's config (invalid OPENAI_API_KEY and a
	// changed DATABASE_URL). Restore the original configuration after the container
	// runs — pass or fail — so other suites that reuse this shared agent aren't left
	// with a broken key. AfterAll always runs once all Its in the Ordered container
	// finish, including when one fails.
	AfterAll(func() {
		deps := deployment.GetDeploymentDetails(Default, Client,
			Cfg.DefaultOrg, SharedITHelpdeskAgent.ProjectName, SharedITHelpdeskAgent.AgentName)
		dep, exists := deps[Cfg.DefaultEnv]
		if !exists || dep.ImageID == "" {
			GinkgoWriter.Printf("config revert: no deployment found, nothing to restore\n")
			return
		}
		before := dep.LastDeployed

		autoInstr := true
		deployment.DeployAgent(Default, Client, Cfg.DefaultOrg, SharedITHelpdeskAgent.ProjectName, SharedITHelpdeskAgent.AgentName,
			framework.DeployAgentRequest{
				ImageID: dep.ImageID,
				Env: []framework.EnvironmentVariable{
					{Key: "OPENAI_API_KEY", Value: Cfg.OpenAIAPIKey, IsSensitive: true},
					{Key: "DATABASE_URL", Value: "http://localhost:5000", IsSensitive: false},
				},
				EnableAutoInstrumentation: &autoInstr,
			})
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:       Cfg.DefaultOrg,
			ProjectName:   SharedITHelpdeskAgent.ProjectName,
			AgentName:     SharedITHelpdeskAgent.AgentName,
			Environment:   Cfg.DefaultEnv,
			Timeout:       10 * time.Minute,
			DeployedAfter: before,
		})
		GinkgoWriter.Printf("config revert: restored original configuration (DATABASE_URL=http://localhost:5000, valid OPENAI_API_KEY)\n")
	})

	It("reports the agent's initial environment configuration", func() {
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, SharedITHelpdeskAgent.ProjectName, SharedITHelpdeskAgent.AgentName, Cfg.DefaultEnv)

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
			Cfg.DefaultOrg, SharedITHelpdeskAgent.ProjectName, SharedITHelpdeskAgent.AgentName)
		dep, exists := deps[Cfg.DefaultEnv]
		Expect(exists).To(BeTrue(), "deployment should exist for default environment")
		Expect(dep.ImageID).NotTo(BeEmpty(), "imageId should not be empty")
		lastDeployedBefore = dep.LastDeployed

		autoInstr := true
		deployment.DeployAgent(Default, Client, Cfg.DefaultOrg, SharedITHelpdeskAgent.ProjectName, SharedITHelpdeskAgent.AgentName,
			framework.DeployAgentRequest{
				ImageID: dep.ImageID,
				Env: []framework.EnvironmentVariable{
					{Key: "OPENAI_API_KEY", Value: "sk-invalid-key-for-e2e-test", IsSensitive: true},
					{Key: "DATABASE_URL", Value: "http://localhost:6000", IsSensitive: false},
				},
				EnableAutoInstrumentation: &autoInstr,
			})
		GinkgoWriter.Printf("Redeployment triggered with updated configurations\n")
	})

	It("becomes active after the configuration redeploy", func() {
		deployment.WaitForDeployed(Client, &deployment.WaitForDeploymentParams{
			OrgName:       Cfg.DefaultOrg,
			ProjectName:   SharedITHelpdeskAgent.ProjectName,
			AgentName:     SharedITHelpdeskAgent.AgentName,
			Environment:   Cfg.DefaultEnv,
			Timeout:       10 * time.Minute,
			DeployedAfter: lastDeployedBefore,
		})
	})

	It("becomes ready after the configuration redeploy", func() {
		deployment.WaitForReadiness(Client, Cfg.DefaultOrg, SharedITHelpdeskAgent.ProjectName, SharedITHelpdeskAgent.AgentName, Cfg.DefaultEnv, 10*time.Minute)
	})

	It("reflects the updated non-secret configuration values", func() {
		configs := configuration.GetAgentConfigurations(Default, Client,
			Cfg.DefaultOrg, SharedITHelpdeskAgent.ProjectName, SharedITHelpdeskAgent.AgentName, Cfg.DefaultEnv)

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
		agentops.WaitForRuntimeLog(Client, &agentops.WaitForRuntimeLogParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: SharedITHelpdeskAgent.ProjectName,
			AgentName:   SharedITHelpdeskAgent.AgentName,
			Environment: Cfg.DefaultEnv,
			SearchText:  "Incorrect API key provided",
			Timeout:     10 * time.Minute,
		})
	})
})
