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

package cliagentplatformtests

import (
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

// H is the shared CLI harness: binary built once, logged in per parallel process.
var H = amctl.RegisterSuite()

// Per-process shared fixture for the platform-agent suite: the loaded config, an
// authenticated API client, and the dedicated CLI-owned agent. The observability
// and agent-llm specs both need these; ensurePlatformAgent provisions them once
// instead of each spec re-running login and agent setup in its own BeforeAll.
var (
	setupOnce sync.Once
	cfg       *framework.Config
	apiClient *framework.AMPClient
	owned     *framework.CLILifecycleAgent
)

func ensurePlatformAgent() {
	setupOnce.Do(func() {
		var err error
		cfg = framework.LoadConfig()
		apiClient, err = framework.NewAMPClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		// Idempotent: builds/provisions the CLI-owned agent only on first call.
		owned = testsetup.SetupCLILifecycleAgent(apiClient, cfg)
	})
}

func TestCLIAgentPlatform(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CLI Platform Agent Suite")
}
