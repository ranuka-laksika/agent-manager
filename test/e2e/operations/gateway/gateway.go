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

package gateway

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// WaitForActiveGatewayForEnv polls the gateways API until the gateway associated
// with the given environment reports status ACTIVE. A freshly created environment's
// gateway takes time to program its routes/auth, so callers should gate any
// invocation through that environment on this readiness check. Returns the
// gateway UUID.
func WaitForActiveGatewayForEnv(client *framework.AMPClient, orgName, envName string, timeout time.Duration) string {
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/gateways", orgName)
	scope := fmt.Sprintf("org=%s env=%s", orgName, envName)

	var lastDiag string
	framework.AttachOnFailure("env-gateway: last poll result", func() string { return lastDiag })

	var gatewayUUID string
	Eventually(func(g Gomega) {
		resp, err := client.Get(path)
		g.Expect(err).NotTo(HaveOccurred(), "list gateways request failed (%s)", scope)
		defer resp.Body.Close()

		// A 4xx is a client/config error (bad scope, auth, missing route) that
		// will not resolve by waiting — fail fast instead of polling to timeout.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			lastDiag = fmt.Sprintf("%s | list gateways returned %d (non-retryable)", scope, resp.StatusCode)
			StopTrying(fmt.Sprintf("list gateways returned %d (%s)", resp.StatusCode, scope)).Now()
		}

		gateways := framework.ExpectStatusAndDecode[framework.GatewayListResponse](g, resp, 200)

		var found bool
		for _, gw := range gateways.Gateways {
			for _, env := range gw.Environments {
				if env.Name == envName {
					lastDiag = fmt.Sprintf("%s | gateway=%s status=%s", scope, gw.Name, gw.Status)
					ginkgo.GinkgoWriter.Printf("Gateway for env %q: %s (UUID: %s, status: %s)\n",
						envName, gw.Name, gw.UUID, gw.Status)
					g.Expect(gw.Status).To(Equal("ACTIVE"), "gateway %q for env %q exists but is not ACTIVE yet (status=%s)", gw.Name, envName, gw.Status)
					gatewayUUID = gw.UUID
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			lastDiag = fmt.Sprintf("%s | no gateway mapped to env among %d gateway(s)", scope, len(gateways.Gateways))
		}
		g.Expect(found).To(BeTrue(), "no gateway found for environment %q among %d gateway(s)", envName, len(gateways.Gateways))
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())

	return gatewayUUID
}
