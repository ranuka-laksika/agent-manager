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

// Validates the amctl agent build read commands end to end against the
// CLI-owned platform agent: list, get (by name), and logs (by name and
// latest), plus the not-found error envelopes. These are read-only: they
// assert against the build produced when the CLI-owned agent was provisioned
// and never trigger a new build, so they don't disturb the deploy specs (which
// deploy the agent's latest build). Asserts JSON envelopes and exit codes only.

package cliagentplatformtests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
)

var _ = Describe("amctl agent build (CLI-owned agent, read-only)", Label("cli", "agent", "build"), Ordered, func() {
	// buildName is the CLI-owned agent's existing build, captured once and
	// reused to drive the get/logs specs. The CLI-owned agent is provisioned
	// once by the suite's SynchronizedBeforeSuite (see suite_test.go), so
	// owned/cfg are ready before these specs run — no per-suite setup needed.
	var buildName string

	BeforeAll(func() {
		list := cliagent.ListAgentBuilds(Default, H, H.Org(), owned.ProjectName, owned.AgentName)
		Expect(list.Builds).NotTo(BeEmpty(), "CLI-owned agent should have at least one build from provisioning")
		buildName = list.Builds[0].BuildName
		Expect(buildName).NotTo(BeEmpty())
		GinkgoWriter.Printf("CLI agent build: agent=%s build=%s\n", owned.AgentName, buildName)
	})

	It("lists builds for the agent", func() {
		list := cliagent.ListAgentBuilds(Default, H, H.Org(), owned.ProjectName, owned.AgentName)
		Expect(list.Total).To(BeNumerically(">=", 1))
		Expect(list.Builds).NotTo(BeEmpty())
		for _, b := range list.Builds {
			Expect(b.BuildName).NotTo(BeEmpty())
			Expect(b.AgentName).To(Equal(owned.AgentName))
			Expect(b.ProjectName).To(Equal(owned.ProjectName))
		}
	})

	It("respects --limit when listing builds", func() {
		list := cliagent.ListAgentBuildsPaged(Default, H, H.Org(), owned.ProjectName, owned.AgentName, 1, 0)
		Expect(len(list.Builds)).To(BeNumerically("<=", 1))
		Expect(list.Total).To(BeNumerically(">=", 1))
	})

	It("gets a build by name", func() {
		b := cliagent.GetAgentBuild(Default, H, H.Org(), owned.ProjectName, owned.AgentName, buildName)
		Expect(b.BuildName).To(Equal(buildName))
		Expect(b.AgentName).To(Equal(owned.AgentName))
		Expect(b.ProjectName).To(Equal(owned.ProjectName))
		Expect(b.Status).NotTo(BeEmpty())
		Expect(b.StartedAt).NotTo(BeZero())
		Expect(b.ImageId).NotTo(BeEmpty(), "a deployed agent's build should carry an image id")
	})

	It("returns build logs for a named build", func() {
		logs := cliagent.GetAgentBuildLogs(Default, H, H.Org(), owned.ProjectName, owned.AgentName, buildName)
		Expect(logs.Logs).NotTo(BeEmpty(), "expected build logs for a completed build")
		Expect(logs.TotalCount).To(BeNumerically(">=", 1))
		for _, entry := range logs.Logs {
			Expect(entry.Log).NotTo(BeEmpty())
		}
	})

	It("returns logs for the latest build when no build name is given", func() {
		logs := cliagent.GetAgentLatestBuildLogs(Default, H, H.Org(), owned.ProjectName, owned.AgentName)
		Expect(logs.Logs).NotTo(BeEmpty())
	})

	It("reports not-found for an unknown build (get)", func() {
		e := cliagent.GetAgentBuildExpectError(Default, H, H.Org(), owned.ProjectName, owned.AgentName, "build-nonexistent-00000000")
		Expect(e.Status).To(Equal(404))
	})

	It("reports not-found for an unknown build (logs)", func() {
		e := cliagent.GetAgentBuildLogsExpectError(Default, H, H.Org(), owned.ProjectName, owned.AgentName, "build-nonexistent-00000000")
		Expect(e.Status).To(Equal(404))
	})
})
