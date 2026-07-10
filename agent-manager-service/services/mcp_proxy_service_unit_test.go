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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
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
// given upstream URL and security.
func endpointWith(url string, security *models.SecurityConfig) models.MCPProxyEndpointDTO {
	var upstream models.UpstreamConfig
	if url != "" {
		upstream.Main = &models.UpstreamEndpoint{URL: url}
	}
	return models.MCPProxyEndpointDTO{
		ID:           "primary",
		Upstream:     upstream,
		Security:     security,
		Environments: []models.MCPEndpointEnvironmentDTO{{EnvironmentUUID: testMCPEnvUUID}},
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
		}),
	}
	err := validateMCPEndpoints(context.Background(), endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpoints_RejectsDuplicateEnvironmentAcrossEndpoints(t *testing.T) {
	first := endpointWith("https://93.184.216.34", nil)
	first.ID = "primary"
	second := endpointWith("https://93.184.216.35", nil)
	second.ID = "secondary"
	err := validateMCPEndpoints(context.Background(), []models.MCPProxyEndpointDTO{first, second})
	assert.ErrorIs(t, err, utils.ErrMCPEnvAlreadyBound)
}

func TestValidateMCPEndpointSecurity_IdentityNeedsGatewayPolicies(t *testing.T) {
	// Gateway advertises mcp-auth but not mcp-authz.
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1")}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	err := svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestValidateMCPEndpointSecurity_IdentityAcceptedWithGatewayPolicies(t *testing.T) {
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gatewayWithPolicyManifest("mcp-auth", "v1", "mcp-authz", "v1")}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints))
}

func TestValidateMCPEndpointSecurity_IdentityAllowedWhenNoGatewayYet(t *testing.T) {
	// No active gateway for the environment yet: identity mode is allowed; policies
	// are re-checked once a gateway exists.
	gwRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}
	svc := &MCPProxyService{gatewayRepo: gwRepo}
	endpoints := []models.MCPProxyEndpointDTO{
		endpointWith("https://93.184.216.34", identityEnabledSecurity()),
	}
	assert.NoError(t, svc.validateMCPEndpointSecurity(context.Background(), "org1", endpoints))
}

// newDeleteTestProxy builds a proxy with one identity-enabled endpoint bound to envUUID,
// ready for MCPProxyService.Delete's Thunder cleanup.
func newDeleteTestProxy(handle string, envUUID uuid.UUID) *models.MCPProxy {
	return &models.MCPProxy{
		UUID:      uuid.New(),
		Artifact:  &models.Artifact{Handle: handle},
		Endpoints: []models.MCPProxyEndpoint{identityEnabledEndpoint("primary", envUUID, uuid.Nil, true)},
	}
}

func TestMCPProxyDelete_CleansThunderResourceServers(t *testing.T) {
	envUUID := uuid.New()
	proxy := newDeleteTestProxy("gh-proxy", envUUID)

	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) { return proxy, nil },
		DeleteFunc:      func(_ context.Context, _, _ string) error { return nil },
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		ListEndpointEnvironmentsByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error) {
			return nil, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{Action: "read"}, {Action: "write"}}, nil
		},
	}
	infra := stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{Name: "env-a", UUID: envUUID.String()}}, nil
	}}

	var deletedHandle string
	type removedPermission struct {
		roleID string
		req    thundersvc.RolePermissionRequest
	}
	var removed []removedPermission
	envClient := &clientmocks.EnvIdentityClientMock{
		DeleteProxyResourceServerFunc: func(_ context.Context, proxyHandle string) error {
			deletedHandle = proxyHandle
			return nil
		},
		ListRolesFunc: func(_ context.Context, ouID string, offset, _ int) ([]thundersvc.ThunderRole, int, error) {
			assert.Equal(t, "", ouID, "role sweep must list every role in the env-Thunder, not filter by a platform OU")
			if offset > 0 {
				return nil, 1, nil
			}
			return []thundersvc.ThunderRole{
				{ID: "role-1", Permissions: []thundersvc.RolePermissionRequest{
					{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:read", "gh-proxy:write"}},
				}},
			}, 1, nil
		},
		RemoveRolePermissionsFunc: func(_ context.Context, roleID string, req thundersvc.RolePermissionRequest) error {
			removed = append(removed, removedPermission{roleID: roleID, req: req})
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, orgName, envName string) (thundersvc.EnvIdentityClient, error) {
			assert.Equal(t, "org", orgName)
			assert.Equal(t, "env-a", envName)
			return envClient, nil
		},
	}

	svc := &MCPProxyService{
		repo:              proxyRepo,
		endpointRepo:      endpointRepo,
		mcpProxyScopeRepo: scopeRepo,
		infraManager:      infra,
		resolver:          resolver,
		logger:            discardLogger(),
	}

	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy")

	assert.NoError(t, err)
	assert.Equal(t, "gh-proxy", deletedHandle)
	if assert.Len(t, removed, 2) {
		assert.Equal(t, thundersvc.RolePermissionRequest{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:read"}}, removed[0].req)
		assert.Equal(t, thundersvc.RolePermissionRequest{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:write"}}, removed[1].req)
	}
}

func TestMCPProxyDelete_CleanupSurvivesResolverError(t *testing.T) {
	envUUID := uuid.New()
	proxy := newDeleteTestProxy("gh-proxy", envUUID)

	proxyRepo := &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(_ context.Context, _, _ string) (*models.MCPProxy, error) { return proxy, nil },
		DeleteFunc:      func(_ context.Context, _, _ string) error { return nil },
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		ListEndpointEnvironmentsByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error) {
			return nil, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyFunc: func(_ context.Context, _ uuid.UUID) ([]models.MCPProxyScope, error) {
			return []models.MCPProxyScope{{Action: "read"}}, nil
		},
	}
	infra := stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{Name: "env-a", UUID: envUUID.String()}}, nil
	}}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(_ context.Context, _, _ string) (thundersvc.EnvIdentityClient, error) {
			return nil, assert.AnError
		},
	}

	svc := &MCPProxyService{
		repo:              proxyRepo,
		endpointRepo:      endpointRepo,
		mcpProxyScopeRepo: scopeRepo,
		infraManager:      infra,
		resolver:          resolver,
		logger:            discardLogger(),
	}

	err := svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy")

	assert.NoError(t, err, "Thunder cleanup is best-effort and must never fail the delete")
}
