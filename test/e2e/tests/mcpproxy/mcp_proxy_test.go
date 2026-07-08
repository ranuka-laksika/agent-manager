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

// Validates the MCP proxy lifecycle: discovering a live upstream MCP server,
// creating a proxy from it (with a per-environment blueprint block deployed to
// that environment's AI gateway), reading it back by handle, listing, and
// deleting it.

package mcpproxy

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	mcpproxyop "github.com/wso2/agent-manager/test/e2e/operations/mcpproxy"
)

var _ = Describe("MCP Proxy Lifecycle", Label("mcp-proxy"), Ordered, func() {
	var (
		suffix      string
		proxyID     string
		gatewayUUID string
		envUUID     string
	)

	BeforeAll(func() {
		suffix = uuid.New().String()[:8]
		proxyID = "e2e-test-mcp-proxy-" + suffix
	})

	It("should have a running AI gateway", func() {
		gatewayUUID, envUUID = gateway.WaitForActiveGatewayForEnvWithEnvUUID(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		Expect(gatewayUUID).NotTo(BeEmpty())
		Expect(envUUID).NotTo(BeEmpty(), "expected an environment UUID for %s", Cfg.DefaultEnv)
	})

	It("should discover the upstream MCP server", func() {
		info := mcpproxyop.FetchServerInfo(Default, Client, Cfg.DefaultOrg,
			framework.MCPServerInfoFetchRequest{
				URL: framework.TestMCPServerURL,
			})
		Expect(len(info.Tools)).To(BeNumerically(">", 0), "expected the everything server to report tools")
		GinkgoWriter.Printf("Discovered MCP server: %d tools, %d prompts, %d resources\n",
			len(info.Tools), len(info.Prompts), len(info.Resources))
	})

	It("should create an MCP proxy with a per-environment block deployed to the gateway", func() {
		upstreamURL := framework.TestMCPServerURL
		ctx := "/" + proxyID

		proxy := mcpproxyop.CreateMCPProxy(Default, Client, Cfg.DefaultOrg,
			framework.CreateMCPProxyRequest{
				ID:      proxyID,
				Name:    "E2E MCP Proxy " + suffix,
				Version: "v1.0",
				Context: &ctx,
				// The proxy is a per-environment blueprint keyed by environment UUID; the
				// block for DefaultEnv deploys a gateway artifact to that env's gateway.
				Environments: map[string]framework.MCPEnvironmentConfig{
					envUUID: {
						Upstream: &framework.UpstreamEndpoint{URL: &upstreamURL},
						Security: &framework.SecurityConfig{
							Enabled: true,
							APIKey: &framework.SecurityAPIKey{
								Enabled: true,
								Key:     "X-API-Key",
								In:      "header",
							},
						},
					},
				},
			})
		Expect(proxy.ID).To(Equal(proxyID))
		GinkgoWriter.Printf("Created MCP proxy %s (env %s deployed to gateway %s)\n", proxy.ID, envUUID, gatewayUUID)
	})

	It("should get the MCP proxy by handle", func() {
		proxy := mcpproxyop.GetMCPProxy(Default, Client, Cfg.DefaultOrg, proxyID)
		Expect(proxy.ID).To(Equal(proxyID))
		Expect(proxy.Name).To(Equal("E2E MCP Proxy " + suffix))
		Expect(proxy.Environments).To(HaveKey(envUUID), "expected a per-environment block for %s", envUUID)
		env := proxy.Environments[envUUID]
		Expect(env.Upstream).NotTo(BeNil(), "expected an upstream in the environment block")
		Expect(env.Upstream.URL).NotTo(BeNil())
		Expect(*env.Upstream.URL).To(Equal(framework.TestMCPServerURL))
	})

	It("should list the MCP proxy", func() {
		list := mcpproxyop.ListMCPProxies(Default, Client, Cfg.DefaultOrg)
		found := false
		for _, p := range list.List {
			if p.ID != nil && *p.ID == proxyID {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "expected created proxy %s in the list", proxyID)
	})

	It("should delete the MCP proxy and then return 404", func() {
		mcpproxyop.DeleteMCPProxy(Default, Client, Cfg.DefaultOrg, proxyID)
		mcpproxyop.GetMCPProxyExpectStatus(Default, Client, Cfg.DefaultOrg, proxyID, 404)
	})
})
