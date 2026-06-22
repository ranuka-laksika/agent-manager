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

package agent

import (
	"fmt"
	"net/http"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// PromoteAgent promotes an agent from a source environment to a target environment
// and asserts a 202 response.
func PromoteAgent(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.PromoteAgentRequest) framework.PromoteAgentResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/promote",
		orgName, projName, agentName)
	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "promote agent request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.PromoteAgentResponse](g, resp, http.StatusAccepted)
}
