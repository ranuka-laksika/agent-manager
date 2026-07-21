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

package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func gatewayRuntimeTestConfig() config.GatewayRuntimeConfig {
	return config.GatewayRuntimeConfig{
		NamePrefix:    "api-platform-",
		ServiceSuffix: "-gateway-gateway-runtime",
		Port:          22893,
	}
}

func TestGatewayRuntimeInClusterURL(t *testing.T) {
	tests := []struct {
		name     string
		gateway  *models.Gateway
		expected string
	}{
		{
			name: "derives runtime service and namespace",
			gateway: &models.Gateway{
				Name: "api-platform-acme-dev",
			},
			expected: "http://api-platform-acme-dev-gateway-gateway-runtime.acme-dev:22893",
		},
		{
			name: "trims gateway name",
			gateway: &models.Gateway{
				Name: " api-platform-acme-prod ",
			},
			expected: "http://api-platform-acme-prod-gateway-gateway-runtime.acme-prod:22893",
		},
		{
			name: "custom name falls back to vhost",
			gateway: &models.Gateway{
				Name: "custom-gateway",
			},
			expected: "",
		},
		{
			name: "empty derived namespace falls back to vhost",
			gateway: &models.Gateway{
				Name: gatewayRuntimeTestConfig().NamePrefix,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, gatewayRuntimeInClusterURL(tt.gateway, gatewayRuntimeTestConfig()))
		})
	}
}

func TestGatewayRuntimeInClusterURLUsesConfiguredValues(t *testing.T) {
	gateway := &models.Gateway{Name: "custom-acme-dev"}
	runtimeConfig := config.GatewayRuntimeConfig{
		NamePrefix:    "custom-",
		ServiceSuffix: "-runtime",
		Port:          9443,
	}

	require.Equal(t, "http://custom-acme-dev-runtime.acme-dev:9443", gatewayRuntimeInClusterURL(gateway, runtimeConfig))
}

func TestBuildProxyURLSelectsReachableGateway(t *testing.T) {
	contextPath := "/llm/proxy"
	gateway := &models.Gateway{
		Name:  "api-platform-acme-dev",
		Vhost: "https://dev-acme.gateway.example.com",
	}

	require.Equal(
		t,
		"http://api-platform-acme-dev-gateway-gateway-runtime.acme-dev:22893/llm/proxy",
		buildProxyURL(gateway, &contextPath, true, gatewayRuntimeTestConfig()),
	)
	require.Equal(
		t,
		"https://dev-acme.gateway.example.com/llm/proxy",
		buildProxyURL(gateway, &contextPath, false, gatewayRuntimeTestConfig()),
	)
}

func TestBuildMCPProxyURLSelectsReachableGateway(t *testing.T) {
	contextPath := "  /tools/  "
	gateway := &models.Gateway{
		Name:  "api-platform-acme-dev",
		Vhost: "https://dev-acme.gateway.example.com/",
	}

	require.Equal(
		t,
		"http://api-platform-acme-dev-gateway-gateway-runtime.acme-dev:22893/tools/mcp",
		buildMCPProxyURL(gateway, &contextPath, true, gatewayRuntimeTestConfig()),
	)
	require.Equal(
		t,
		"https://dev-acme.gateway.example.com/tools/mcp",
		buildMCPProxyURL(gateway, &contextPath, false, gatewayRuntimeTestConfig()),
	)
}

func TestBuildMCPProxyURLFallsBackToVhostForCustomGatewayName(t *testing.T) {
	gateway := &models.Gateway{
		Name:  "custom-gateway",
		Vhost: "https://gateway.example.com/",
	}

	require.Equal(t, "https://gateway.example.com/mcp", buildMCPProxyURL(gateway, nil, true, gatewayRuntimeTestConfig()))
}
