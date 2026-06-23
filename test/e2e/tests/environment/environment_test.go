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

// Validates the environment lifecycle: provisioning a new environment via the
// add-environment script and removing it via the remove-environment script.

package environment

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	envops "github.com/wso2/agent-manager/test/e2e/operations/environment"
)

var _ = Describe("Environment management:", Label("environment"), Ordered, func() {
	var (
		envName      string
		scriptParams *envops.ScriptParams
	)

	BeforeAll(func() {
		suffix := uuid.New().String()[:6]
		envName = framework.E2EEnvPrefix + suffix
		scriptParams = envops.FromClient(Client)
		scriptParams.EnvName = envName
		scriptParams.DisplayName = "E2E Env " + suffix
	})

	It("creates an environment via the add-environment script", func() {
		envops.AddEnvironment(scriptParams)

		env := envops.GetEnvironment(Default, Client, Cfg.DefaultOrg, envName)
		Expect(env.Name).To(Equal(envName))
		GinkgoWriter.Printf("Environment created: %s\n", envName)
	})

	It("removes the environment via the remove-environment script", func() {
		err := envops.RemoveEnvironment(scriptParams)
		Expect(err).NotTo(HaveOccurred(), "remove-environment.sh failed for env %q", envName)
		GinkgoWriter.Printf("Environment removed: %s\n", envName)
	})
})
