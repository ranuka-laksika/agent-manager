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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// testMCPEnvUUID is a valid environment UUID used as an endpoint's target environment.
const testMCPEnvUUID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"

// identityEnabledSecurity returns a SecurityConfig selecting the Agent Identity variant.
func identityEnabledSecurity() *models.SecurityConfig {
	return &models.SecurityConfig{
		Enabled:  boolPtr(true),
		Identity: &models.IdentitySecurity{Enabled: boolPtr(true)},
	}
}

// endpointWith builds a single-environment endpoint DTO targeting testMCPEnvUUID with the
// given upstream URL, security, and tool-scope bindings.
func endpointWith(url string, security *models.SecurityConfig, bindings []models.MCPToolScopeBinding) models.MCPProxyEndpointDTO {
	var upstream models.UpstreamConfig
	if url != "" {
		upstream.Main = &models.UpstreamEndpoint{URL: url}
	}
	return models.MCPProxyEndpointDTO{
		ID:                "primary",
		Upstream:          upstream,
		Security:          security,
		ToolScopeBindings: bindings,
		Environments:      []models.MCPEndpointEnvironmentDTO{{EnvironmentUUID: testMCPEnvUUID}},
	}
}

// gatewayWithPolicyManifest builds a Gateway whose Manifest advertises the given
// name/version policy pairs, in the shape extractGatewayPolicyManifestItems walks.
func gatewayWithPolicyManifest(nameVersionPairs ...string) *models.Gateway {
	items := make([]interface{}, 0, len(nameVersionPairs)/2)
	for i := 0; i+1 < len(nameVersionPairs); i += 2 {
		items = append(items, map[string]interface{}{
			"name":    nameVersionPairs[i],
			"version": nameVersionPairs[i+1],
		})
	}
	return &models.Gateway{Manifest: map[string]interface{}{"policies": items}}
}

func TestDefaultMCPProxySecurity_IdentityVariantSkipsAPIKeyDefaults(t *testing.T) {
	out := defaultMCPProxySecurity(&models.SecurityConfig{
		Enabled:  boolPtr(true),
		Identity: &models.IdentitySecurity{Enabled: boolPtr(true)},
	})
	assert.Nil(t, out.APIKey, "identity mode must not default an API key on")
	assert.NotNil(t, out.Identity)
	assert.True(t, isBoolTrue(out.Identity.Enabled))
}

func TestMCPProxyCreate_RejectsNonKebabHandle(t *testing.T) {
	svc := &MCPProxyService{}
	for _, id := range []string{"Bad_Handle", "UPPER", "has space", "trail-", "-lead", "a--b", strings.Repeat("a", 101)} {
		_, err := svc.Create(context.Background(), "org-uuid", "system",
			&models.MCPProxyDTO{ID: id, Name: "x", Version: "v1"})
		assert.ErrorIs(t, err, utils.ErrInvalidInput, "handle %q must be rejected", id)
		assert.Contains(t, err.Error(), "kebab-case", "handle %q must be rejected by the kebab check, not a later validation", id)
	}
}

func TestValidateMCPEndpoints_RejectsBothVariantsEnabled(t *testing.T) {
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", &models.SecurityConfig{
			Enabled:  boolPtr(true),
			APIKey:   &models.APIKeySecurity{Enabled: boolPtr(true)},
			Identity: &models.IdentitySecurity{Enabled: boolPtr(true)},
		}, nil),
	}
	err := validateMCPEndpoints(context.Background(), endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpoints_RejectsBindingWithNoScopes(t *testing.T) {
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity(), []models.MCPToolScopeBinding{
			{Tool: "list_repos", Scopes: nil},
		}),
	}
	err := validateMCPEndpoints(context.Background(), endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpoints_RejectsDuplicateToolBinding(t *testing.T) {
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity(), []models.MCPToolScopeBinding{
			{Tool: "search", Scopes: []string{"a:read"}},
			{Tool: "search", Scopes: []string{"a:admin"}},
		}),
	}
	err := validateMCPEndpoints(context.Background(), endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpoints_RejectsDuplicateEnvironmentAcrossEndpoints(t *testing.T) {
	first := endpointWith("https://93.184.216.34", nil, nil)
	first.ID = "primary"
	second := endpointWith("https://93.184.216.35", nil, nil)
	second.ID = "secondary"
	err := validateMCPEndpoints(context.Background(), []models.MCPProxyEndpointDTO{first, second})
	assert.ErrorIs(t, err, utils.ErrMCPEnvAlreadyBound)
}

func TestValidateMCPEndpointSecurity_UnknownBindingScope(t *testing.T) {
	scopeRepo := &repomocks.ScopeRepositoryMock{
		ListFunc: func(_ context.Context, orgName string) ([]models.Scope, error) {
			return []models.Scope{{OrgName: orgName, Name: "repo:read.all"}}, nil
		},
	}
	svc := &MCPProxyService{scopeRepo: scopeRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity(), []models.MCPToolScopeBinding{
			{Tool: "create_issue", Scopes: []string{"repo:write.all"}},
		}),
	}
	err := svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "repo:write.all")
}

func TestValidateMCPEndpointSecurity_KnownScopesPass(t *testing.T) {
	scopeRepo := &repomocks.ScopeRepositoryMock{
		ListFunc: func(_ context.Context, _ string) ([]models.Scope, error) {
			return []models.Scope{{Name: "repo:read.all"}}, nil
		},
	}
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1", "mcp-authz", "v1")}, nil
		},
	}
	svc := &MCPProxyService{scopeRepo: scopeRepo, gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity(), []models.MCPToolScopeBinding{
			{Tool: "list_repos", Scopes: []string{"repo:read.all"}},
		}),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints))
}

func TestValidateMCPEndpointSecurity_IdentityNeedsGatewayPolicies(t *testing.T) {
	scopeRepo := &repomocks.ScopeRepositoryMock{
		ListFunc: func(_ context.Context, _ string) ([]models.Scope, error) {
			return []models.Scope{{Name: "repo:read.all"}}, nil
		},
	}
	// Gateway advertises mcp-auth but not mcp-authz.
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1")}, nil
		},
	}
	svc := &MCPProxyService{scopeRepo: scopeRepo, gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity(), []models.MCPToolScopeBinding{
			{Tool: "list_repos", Scopes: []string{"repo:read.all"}},
		}),
	}
	err := svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpointSecurity_IdentityAcceptedWithGatewayPolicies(t *testing.T) {
	scopeRepo := &repomocks.ScopeRepositoryMock{
		ListFunc: func(_ context.Context, _ string) ([]models.Scope, error) {
			return []models.Scope{{Name: "repo:read.all"}}, nil
		},
	}
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1", "mcp-authz", "v1")}, nil
		},
	}
	svc := &MCPProxyService{scopeRepo: scopeRepo, gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity(), []models.MCPToolScopeBinding{
			{Tool: "list_repos", Scopes: []string{"repo:read.all"}},
		}),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints))
}

func TestValidateMCPEndpointSecurity_IdentityAllowedWhenNoGatewayYet(t *testing.T) {
	scopeRepo := &repomocks.ScopeRepositoryMock{
		ListFunc: func(_ context.Context, _ string) ([]models.Scope, error) {
			return []models.Scope{{Name: "repo:read.all"}}, nil
		},
	}
	// No active gateway for the environment yet: identity mode is allowed; policies
	// are re-checked once a gateway exists.
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}
	svc := &MCPProxyService{scopeRepo: scopeRepo, gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity(), []models.MCPToolScopeBinding{
			{Tool: "list_repos", Scopes: []string{"repo:read.all"}},
		}),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints))
}
