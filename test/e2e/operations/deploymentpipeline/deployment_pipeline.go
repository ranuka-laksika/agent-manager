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

// Package deploymentpipeline provides e2e operations for org deployment pipelines.
package deploymentpipeline

import (
	"fmt"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Create creates an org deployment pipeline and asserts a 201 response.
func Create(g Gomega, client *framework.AMPClient, orgName string, req framework.CreateDeploymentPipelineRequest) framework.DeploymentPipelineResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", orgName)
	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "create deployment pipeline request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.DeploymentPipelineResponse](g, resp, http.StatusCreated)
}

// CreateOrGet creates an org deployment pipeline, tolerating a 409 when the
// pipeline already exists. A prior run (or a half-provisioned promotable project
// whose pipeline was created before the project create failed) can leave the
// pipeline behind, so a strict 201 assertion would spuriously fail subsequent
// suites. On 409 the existing pipeline is looked up by display name and returned.
func CreateOrGet(g Gomega, client *framework.AMPClient, orgName string, req framework.CreateDeploymentPipelineRequest) framework.DeploymentPipelineResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", orgName)
	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "create deployment pipeline request failed")
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		existing := findByDisplayName(g, client, orgName, req.DisplayName)
		g.Expect(existing).NotTo(BeNil(),
			"pipeline create returned 409 but no pipeline with display name %q was found", req.DisplayName)
		return *existing
	}
	return framework.ExpectStatusAndDecode[framework.DeploymentPipelineResponse](g, resp, http.StatusCreated)
}

// findByDisplayName lists org deployment pipelines and returns the one whose
// display name matches, or nil if none match.
func findByDisplayName(g Gomega, client *framework.AMPClient, orgName, displayName string) *framework.DeploymentPipelineResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", orgName)
	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list deployment pipelines request failed")
	defer resp.Body.Close()

	list := framework.ExpectStatusAndDecode[framework.DeploymentPipelineListResponse](g, resp, http.StatusOK)
	for i := range list.DeploymentPipelines {
		if list.DeploymentPipelines[i].DisplayName == displayName {
			return &list.DeploymentPipelines[i]
		}
	}
	return nil
}

// Delete deletes an org deployment pipeline. Intended for cleanup; a 404 is
// tolerated so cleanup is idempotent.
func Delete(client *framework.AMPClient, orgName, pipelineName string) {
	defer ginkgo.GinkgoRecover()
	path := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/%s", orgName, pipelineName)
	resp, err := client.Delete(path)
	Expect(err).NotTo(HaveOccurred(), "delete deployment pipeline request failed")
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(BeElementOf(http.StatusNoContent, http.StatusNotFound),
		"unexpected status deleting deployment pipeline %q", pipelineName)
}
