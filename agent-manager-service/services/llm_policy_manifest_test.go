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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// gatewayWithLLMPolicyManifest builds a Gateway whose Manifest advertises the given
// policies, in the shape the gateway-controller actually pushes ({"policies": [...]}).
func gatewayWithLLMPolicyManifest(policies ...map[string]interface{}) *models.Gateway {
	items := make([]interface{}, 0, len(policies))
	for _, p := range policies {
		items = append(items, p)
	}
	return &models.Gateway{Manifest: map[string]interface{}{"policies": items}}
}

// -----------------------------------------------------------------------------
// extractLLMPolicyManifestItems — the manifest walk itself.
// -----------------------------------------------------------------------------

func TestExtractLLMPolicyManifestItems_FullFieldExtraction(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{
				"name":        "word-count-guardrail",
				"version":     "v1.0.0",
				"displayName": "Word Count Guardrail",
				"description": "Validates word count.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"min": map[string]interface{}{"type": "integer"},
					},
				},
				"systemParameters": map[string]interface{}{
					"type": "object",
				},
			},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "word-count-guardrail", item.Name)
	assert.Equal(t, "v1.0.0", item.Version)
	assert.Equal(t, "Word Count Guardrail", item.DisplayName)
	assert.Equal(t, "Validates word count.", item.Description)
	require.NotNil(t, item.Parameters)
	assert.Equal(t, "object", item.Parameters["type"])
	require.NotNil(t, item.SystemParameters)
	assert.Equal(t, "object", item.SystemParameters["type"])
}

func TestExtractLLMPolicyManifestItems_ToleratesKeyAliases(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{"policyName": "aliased-by-policyname", "version": "v1"},
			map[string]interface{}{"id": "aliased-by-id", "version": "v1"},
			map[string]interface{}{"name": "aliased-version", "policyVersion": "v2"},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	assert.ElementsMatch(t, []string{"aliased-by-policyname", "aliased-by-id", "aliased-version"}, names)
}

func TestExtractLLMPolicyManifestItems_ExpandsVersionsArray(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{
				"name":     "multi-version-policy",
				"versions": []interface{}{"v1", "v2"},
			},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	versions := make([]string, 0, len(items))
	for _, item := range items {
		assert.Equal(t, "multi-version-policy", item.Name)
		versions = append(versions, item.Version)
	}
	assert.ElementsMatch(t, []string{"v1", "v2"}, versions)
}

func TestExtractLLMPolicyManifestItems_IgnoresCoincidentalNameVersionInSchema(t *testing.T) {
	// A real policy whose parameter/system schemas embed nested objects that happen
	// to carry their own name+version string pairs (e.g. a default/example model).
	// Only the top-level policy must surface — the schema leaves must not.
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{
				"name":    "model-round-robin",
				"version": "v1.0.2",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"defaultModel": map[string]interface{}{
							"name":    "gpt-4",
							"version": "1.0",
						},
					},
				},
				"systemParameters": map[string]interface{}{
					"example": map[string]interface{}{
						"name":    "nested-example",
						"version": "9.9",
					},
				},
			},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	require.Len(t, items, 1)
	assert.Equal(t, "model-round-robin", items[0].Name)
	assert.Equal(t, "v1.0.2", items[0].Version)
}

func TestExtractLLMPolicyManifestItems_EmptyOrMalformedManifest(t *testing.T) {
	assert.Empty(t, extractLLMPolicyManifestItems(nil))
	assert.Empty(t, extractLLMPolicyManifestItems(map[string]interface{}{}))
	assert.Empty(t, extractLLMPolicyManifestItems("not a map"))
}

// -----------------------------------------------------------------------------
// intersectActiveGatewayLLMPolicies — active-gateway intersection semantics.
// -----------------------------------------------------------------------------

func TestIntersectActiveGatewayLLMPolicies_NilRepoReturnsEmpty(t *testing.T) {
	available, err := intersectActiveGatewayLLMPolicies(nil, "org-uuid")

	require.NoError(t, err)
	assert.NotNil(t, available)
	assert.Empty(t, available)
}

func TestIntersectActiveGatewayLLMPolicies_SingleGateway(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(map[string]interface{}{"name": "word-count-guardrail", "version": "v1"}),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Contains(t, available, "word-count-guardrail\x00v1")
}

func TestIntersectActiveGatewayLLMPolicies_OverlappingPoliciesSurvive(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-policy", "version": "v1"},
					map[string]interface{}{"name": "only-on-first", "version": "v1"},
				),
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-policy", "version": "v1"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// Only the policy reported by EVERY active gateway survives the intersection.
	require.Len(t, available, 1)
	assert.Contains(t, available, "shared-policy\x00v1")
	assert.NotContains(t, available, "only-on-first\x00v1")
}

func TestIntersectActiveGatewayLLMPolicies_OnlyActiveGatewaysQueried(t *testing.T) {
	var capturedFilters repositories.GatewayFilterOptions
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(filters repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			capturedFilters = filters
			return []*models.Gateway{}, nil
		},
	}

	_, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	assert.Equal(t, "org-uuid", capturedFilters.OrganizationID)
	require.NotNil(t, capturedFilters.Status)
	assert.True(t, *capturedFilters.Status)
}

func TestIntersectActiveGatewayLLMPolicies_RepoErrorIsWrapped(t *testing.T) {
	boom := errors.New("db unreachable")
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return nil, boom
		},
	}

	_, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// -----------------------------------------------------------------------------
// sortedLLMPolicyManifestItems — stable output ordering for the API response.
// -----------------------------------------------------------------------------

func TestSortedLLMPolicyManifestItems_OrdersByNameThenVersion(t *testing.T) {
	available := map[string]llmPolicyManifestItem{
		"zebra-policy\x00v1": {Name: "zebra-policy", Version: "v1"},
		"alpha-policy\x00v2": {Name: "alpha-policy", Version: "v2"},
		"alpha-policy\x00v1": {Name: "alpha-policy", Version: "v1"},
	}

	sorted := sortedLLMPolicyManifestItems(available)

	require.Len(t, sorted, 3)
	assert.Equal(t, "alpha-policy", sorted[0].Name)
	assert.Equal(t, "v1", sorted[0].Version)
	assert.Equal(t, "alpha-policy", sorted[1].Name)
	assert.Equal(t, "v2", sorted[1].Version)
	assert.Equal(t, "zebra-policy", sorted[2].Name)
}
