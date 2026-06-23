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

// Package configuration holds e2e tests for the agent configuration domain:
// redeploying with modified environment variables, verifying non-secret config
// changes, and detecting invalid secrets. These tests deliberately break the
// agent's config (a bad OPENAI_API_KEY) and redeploy, so the suite builds its
// OWN dedicated, disposable agent rather than reusing the shared one — a wedged
// config redeploy must not poison a fixture other suites depend on.

package configuration

import (
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

// Client is the shared API client used by all configuration tests.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

// ConfigAgent is this suite's own disposable IT helpdesk agent. It is uniquely
// named per run so a broken/wedged config never affects other suites.
var ConfigAgent *framework.SharedITHelpdeskAgent

func TestConfiguration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Configuration Suite")
}

// The dedicated agent is provisioned once (on a single process) and decoded by
// every process; see testsetup.SynchronizedITHelpdeskAgent.
var _ = SynchronizedBeforeSuite(
	testsetup.SynchronizedITHelpdeskAgent(
		func(client *framework.AMPClient, cfg *framework.Config) *framework.SharedITHelpdeskAgent {
			name := framework.E2EAgentPrefix + uuid.New().String()[:8]
			return testsetup.SetupITHelpdeskAgent(client, cfg, name,
				"Dedicated, disposable IT helpdesk agent for the configuration suite")
		},
		&Cfg, &Client, &ConfigAgent,
	),
)
