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

package cliagentobstests

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

var _ = Describe("amctl agent observability (CLI-owned lifecycle)", Label("cli", "agent", "slow"), Ordered, func() {
	var (
		cfg   *framework.Config
		owned *framework.CLILifecycleAgent
	)

	BeforeAll(func() {
		if os.Getenv("OPENAI_API_KEY") == "" {
			Skip("OPENAI_API_KEY not set; skipping slow CLI observability suite")
		}
		cfg = framework.LoadConfig()
		client, err := framework.NewAMPClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		// Provision (idempotent, build-once) the dedicated CLI-owned agent.
		owned = testsetup.SetupCLILifecycleAgent(client, cfg)
	})

	It("redeploys the agent via amctl agent deploy", func() {
		res := cliagent.DeployAgent(Default, H, H.Org(), owned.ProjectName, owned.AgentName)
		Expect(res.Agent).To(Equal(owned.AgentName))
		Expect(res.Build).NotTo(BeEmpty())
		Expect(res.ImageId).NotTo(BeEmpty())
		Expect(res.TargetEnvironment).To(Equal(cfg.DefaultEnv))
	})

	It("becomes ready again after redeploy", func() {
		client, err := framework.NewAMPClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		deployment.WaitForReadiness(client, cfg.DefaultOrg, owned.ProjectName, owned.AgentName, cfg.DefaultEnv, 10*time.Minute)
	})

	It("invokes the agent to generate a fresh trace", func() {
		_ = agentops.InvokeAgentEndpoint(owned.EndpointURL+"/chat", owned.InvokeReq, owned.APIKey)
	})

	It("exposes CPU metrics via amctl agent metrics", func() {
		m := cliagent.AgentMetrics(Default, H, H.Org(), owned.ProjectName, owned.AgentName, cfg.DefaultEnv)
		Expect(m.CPUUsage).NotTo(BeEmpty(), "expected CPU usage metrics")
	})

	It("exposes runtime logs via amctl agent logs", func() {
		Eventually(func(g Gomega) {
			l := cliagent.AgentLogs(g, H, H.Org(), owned.ProjectName, owned.AgentName, cfg.DefaultEnv)
			g.Expect(l.Logs).NotTo(BeEmpty(), "expected runtime logs")
		}).WithTimeout(3 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})

	It("captures traces via amctl agent traces", func() {
		Eventually(func(g Gomega) {
			t := cliagent.AgentTraces(g, H, H.Org(), owned.ProjectName, owned.AgentName, cfg.DefaultEnv)
			g.Expect(t.Traces).NotTo(BeEmpty(), "expected at least one trace")
		}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})
})
