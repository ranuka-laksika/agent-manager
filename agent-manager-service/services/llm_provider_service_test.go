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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// -----------------------------------------------------------------------------
// ListAvailableLLMPolicies — surfaces full gateway-reported guardrail definitions
// for the console, replacing the external policy-hub catalog for LLM guardrails.
// -----------------------------------------------------------------------------

func TestLLMProviderService_ListAvailableLLMPolicies_NilGatewayRepoReturnsEmpty(t *testing.T) {
	svc := &LLMProviderService{}

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(0), resp.Count)
	assert.NotNil(t, resp.List)
	assert.Empty(t, resp.List)
}

func TestLLMProviderService_ListAvailableLLMPolicies_SurfacesFullDefinitions(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(map[string]interface{}{
					"name":        "word-count-guardrail",
					"version":     "v1.0.0",
					"displayName": "Word Count Guardrail",
					"description": "Validates word count.",
					"parameters": map[string]interface{}{
						"type": "object",
					},
				}),
			}, nil
		},
	}
	svc := &LLMProviderService{gatewayRepo: repo}

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid")

	require.NoError(t, err)
	require.Equal(t, int32(1), resp.Count)
	require.Len(t, resp.List, 1)

	def := resp.List[0]
	assert.Equal(t, "word-count-guardrail", def.Name)
	assert.Equal(t, "v1.0.0", def.Version)
	assert.Equal(t, "Word Count Guardrail", def.DisplayName)
	assert.Equal(t, "Validates word count.", def.Description)
	require.NotNil(t, def.Parameters)
	assert.Equal(t, "object", def.Parameters["type"])
}

func TestLLMProviderService_ListAvailableLLMPolicies_IntersectsAcrossActiveGateways(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-guardrail", "version": "v1"},
					map[string]interface{}{"name": "only-on-first-gateway", "version": "v1"},
				),
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-guardrail", "version": "v1"},
				),
			}, nil
		},
	}
	svc := &LLMProviderService{gatewayRepo: repo}

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid")

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "shared-guardrail", resp.List[0].Name)
}
