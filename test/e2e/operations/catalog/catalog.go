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

// Package catalog provides e2e operations for the agent catalog (kinds).
package catalog

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// PublishKind publishes a built agent as a reusable kind in the catalog.
// The buildName must refer to a successfully completed build of the source agent.
func PublishKind(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.PublishKindRequest) framework.PublishKindResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/publish-kind",
		orgName, projName, agentName)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "publish kind request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.PublishKindResponse](g, resp)
}

// KindExists reports whether a kind with the given name is currently present in
// the catalog. It performs a single list call (no polling) and is used to make
// kind publishing idempotent.
func KindExists(client *framework.AMPClient, orgName, kindName string) bool {
	path := fmt.Sprintf("/api/v1/orgs/%s/agent-kinds", orgName)
	resp, err := client.Get(path)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	kinds := framework.DecodeBody[framework.AgentKindListResponse](Default, resp)
	for _, k := range kinds.Kinds {
		if k.Name == kindName {
			return true
		}
	}
	return false
}

// WaitForKindAvailable polls the catalog until the specified kind name appears,
// or the timeout elapses. This is used by the catalog test suite to wait for a
// kind that was published by the promotion test suite in a prior CI step.
func WaitForKindAvailable(client *framework.AMPClient, orgName, kindName string, timeout time.Duration) {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/agent-kinds", orgName)
	scope := fmt.Sprintf("org=%s kind=%s", orgName, kindName)

	var lastDiag string
	framework.AttachOnFailure("agent-kinds: last poll result", func() string { return lastDiag })

	Eventually(func(g Gomega) {
		resp, err := client.Get(path)
		g.Expect(err).NotTo(HaveOccurred(), "list agent kinds request failed (%s)", scope)
		defer resp.Body.Close()

		kinds := framework.ExpectStatusAndDecode[framework.AgentKindListResponse](g, resp, 200)

		var found bool
		names := make([]string, 0, len(kinds.Kinds))
		for _, k := range kinds.Kinds {
			names = append(names, k.Name)
			if k.Name == kindName {
				found = true
				break
			}
		}
		lastDiag = fmt.Sprintf("%s | %d kind(s): %v", scope, len(kinds.Kinds), names)
		g.Expect(found).To(BeTrue(),
			"kind %q not yet found in agent-kinds (%d present: %v) — ensure the promotion suite ran first", kindName, len(kinds.Kinds), names)
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())

	ginkgo.GinkgoWriter.Printf("Kind %q is available\n", kindName)
}
