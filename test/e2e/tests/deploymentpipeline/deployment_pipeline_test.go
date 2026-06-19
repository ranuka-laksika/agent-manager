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

// Validates the deployment pipeline lifecycle: creation and deletion.

package deploymentpipeline

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	dpops "github.com/wso2/agent-manager/test/e2e/operations/deploymentpipeline"
	envops "github.com/wso2/agent-manager/test/e2e/operations/environment"
)

var _ = Describe("Deployment pipeline: with two environment promotion path, then delete", Label("deployment-pipeline"), Ordered, func() {
	var (
		stagingEnv   string
		pipelineName string
	)

	var scriptParams *envops.ScriptParams

	BeforeAll(func() {
		suffix := uuid.New().String()[:8]
		stagingEnv = framework.E2EEnvPrefix + suffix

		scriptParams = envops.FromClient(Client)
		scriptParams.EnvName = stagingEnv
		scriptParams.DisplayName = "E2E Staging " + suffix
		envops.AddEnvironment(scriptParams)
		GinkgoWriter.Printf("Staging environment created: %s\n", stagingEnv)
	})

	AfterAll(func() {
		if scriptParams != nil {
			if err := envops.RemoveEnvironment(scriptParams); err != nil {
				GinkgoWriter.Printf("WARNING: failed to remove environment %q: %v\n", stagingEnv, err)
				return
			}
			GinkgoWriter.Printf("Staging environment removed: %s\n", stagingEnv)
		}
	})

	It("creates a deployment pipeline spanning two environments", func() {
		suffix := uuid.New().String()[:8]
		req := framework.CreateDeploymentPipelineRequest{
			DisplayName: "E2E Pipeline " + suffix,
			PromotionPaths: []framework.PromotionPath{
				{
					SourceEnvironmentRef: Cfg.DefaultEnv,
					TargetEnvironmentRefs: []framework.TargetEnvironmentRef{
						{Name: stagingEnv},
					},
				},
			},
		}
		pipeline := dpops.Create(Default, Client, Cfg.DefaultOrg, req)
		pipelineName = pipeline.Name
		Expect(pipelineName).NotTo(BeEmpty())
		Expect(pipeline.DisplayName).To(Equal(req.DisplayName))
		GinkgoWriter.Printf("Deployment pipeline created: %s (%s -> %s)\n", pipelineName, Cfg.DefaultEnv, stagingEnv)
	})

	It("deletes the deployment pipeline", func() {
		dpops.Delete(Client, Cfg.DefaultOrg, pipelineName)
		GinkgoWriter.Printf("Deployment pipeline deleted: %s\n", pipelineName)
	})
})
