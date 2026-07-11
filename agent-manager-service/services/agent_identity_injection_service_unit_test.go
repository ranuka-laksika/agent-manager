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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// testIdentityOrg is deliberately NOT "default" (the config-pinned Thunder
	// namespace — see ThunderOrgNamespace) so tests can't pass by accident if
	// the token endpoint URL is ever built from ouID again instead of the
	// resolved namespace — a real regression this exact aliasing masked before.
	testIdentityOrg     = "019f4ab9-test-ou-id"
	testIdentityProject = "proj-a"
	testIdentityAgent   = "my-agent"
	testIdentityEnv     = "staging"
	testIdentityKVPath  = "agent-thunder-clients/019f4ab9-test-ou-id/proj-a/staging/my-agent"
)

func completedInternalBinding() *models.AgentThunderClient {
	return &models.AgentThunderClient{
		OUID:             testIdentityOrg,
		ProjectName:      testIdentityProject,
		AgentName:        testIdentityAgent,
		EnvironmentName:  testIdentityEnv,
		ProvisioningType: models.AgentProvisioningTypeInternal,
		Status:           models.AgentThunderStatusCompleted,
		ThunderAgentID:   "thunder-agent-1",
		ThunderClientID:  "client-abc",
		SecretRefPath:    testIdentityKVPath,
	}
}

func identityRepoReturning(binding *models.AgentThunderClient, err error) *repomocks.AgentThunderClientRepositoryMock {
	return &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.AgentThunderClient, error) {
			return binding, err
		},
	}
}

// noMCPConfigRepo returns an AgentConfigurationRepository mock reporting "no
// configuration found" for every agent — the default for tests that aren't
// exercising scope resolution, so they don't also need to stub
// OpenChoreoClient.GetEnvironmentFunc (resolveAgentIdentityScopes short-
// circuits before ever calling it when there's no agent configuration).
func noMCPConfigRepo() *repomocks.AgentConfigurationRepositoryMock {
	return &repomocks.AgentConfigurationRepositoryMock{
		GetByAgentIDFunc: func(_ context.Context, _, _ string) (*models.AgentConfiguration, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
}

// noMCPProxyEndpointRepo returns an MCPProxyEndpointRepository mock with every
// method left unset — any unexpected call panics (moq's default), which is
// exactly right for tests whose scope resolution short-circuits before ever
// needing it (e.g. no agent configuration).
func noMCPProxyEndpointRepo() *repomocks.MCPProxyEndpointRepositoryMock {
	return &repomocks.MCPProxyEndpointRepositoryMock{}
}

func newTestIdentityInjectionService(
	repo *repomocks.AgentThunderClientRepositoryMock,
	oc *clientmocks.OpenChoreoClientMock,
) AgentIdentityInjectionService {
	return NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyEndpointRepo(), oc, "1h", discardLogger())
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_CreatesSecretReferenceAndBuildsVars(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	var createdReq client.CreateSecretReferenceRequest
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, _ string) (*client.SecretReferenceInfo, error) {
			return nil, utils.ErrNotFound
		},
		CreateSecretReferenceFunc: func(_ context.Context, namespace string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			assert.Equal(t, testIdentityOrg, namespace)
			createdReq = req
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	require.Len(t, envVars, 4)

	expectedRefName := AgentIdentitySecretRefName(testIdentityAgent, testIdentityEnv)
	assert.Equal(t, expectedRefName, createdReq.Name)
	assert.Equal(t, testIdentityKVPath, createdReq.KVPath, "SecretReference must point at the EXISTING OpenBao path — no secret duplication")
	assert.Equal(t, []string{thundersvc.AgentSecretKeyClientSecret}, createdReq.SecretKeys)
	assert.Equal(t, testIdentityProject, createdReq.ProjectName)
	assert.Equal(t, testIdentityAgent, createdReq.ComponentName)
	assert.Equal(t, "1h", createdReq.RefreshInterval)
	assert.Empty(t, createdReq.TemplateAnnotations, "plain injection must not stamp a rotated-at annotation")

	byKey := map[string]client.EnvVar{}
	for _, ev := range envVars {
		byKey[ev.Key] = ev
	}
	assert.Equal(t, "client-abc", byKey[client.EnvVarAgentIdentityClientID].Value)

	secretVar := byKey[client.EnvVarAgentIdentityClientSecret]
	require.NotNil(t, secretVar.ValueFrom, "client secret must be a SecretKeyRef, never a literal")
	require.NotNil(t, secretVar.ValueFrom.SecretKeyRef)
	assert.Equal(t, expectedRefName, secretVar.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, thundersvc.AgentSecretKeyClientSecret, secretVar.ValueFrom.SecretKeyRef.Key)
	assert.Empty(t, secretVar.Value)

	assert.Equal(t, thundersvc.ThunderTokenURL(ThunderOrgNamespace(), testIdentityEnv), byKey[client.EnvVarAgentIdentityTokenEndpoint].Value,
		"token endpoint must be built from the org's Thunder namespace, NOT the raw ouID")
	assert.Empty(t, byKey[client.EnvVarAgentIdentityScopes].Value, "no agent configuration means no MCP bindings, so no scopes to request")
}

// mcpProxyBinding is one EnvAgentMCPMapping's worth of fixture data: a proxy
// deployed to an environment via an endpoint carrying the given tool-scope
// bindings — the real chain resolveAgentIdentityScopes now walks (proxy ->
// endpoint-environment join -> endpoint -> its own ToolScopeBindings), not a
// map read directly off the proxy.
type mcpProxyBinding struct {
	envUUID  string
	bindings []models.MCPToolScopeBinding
}

// mcpBoundAgentConfigRepo returns an AgentConfigurationRepository mock whose
// GetByAgentID returns a config with one EnvAgentMCPMapping per given binding,
// plus the MCPProxyEndpointRepository mock that resolves each mapping's
// (proxy, environment) to a distinct endpoint carrying its ToolScopeBindings —
// together, what resolveAgentIdentityScopes needs to aggregate scopes.
func mcpBoundAgentConfigRepo(bindings ...mcpProxyBinding) (*repomocks.AgentConfigurationRepositoryMock, *repomocks.MCPProxyEndpointRepositoryMock) {
	type key struct{ proxy, env uuid.UUID }
	endpointByKey := map[key]uuid.UUID{}
	configByEndpoint := map[uuid.UUID]models.MCPEndpointConfig{}
	mappings := make([]models.EnvAgentMCPMapping, 0, len(bindings))

	for _, b := range bindings {
		proxyUUID := uuid.New()
		envUUID := uuid.MustParse(b.envUUID)
		endpointUUID := uuid.New()
		mappings = append(mappings, models.EnvAgentMCPMapping{EnvironmentUUID: envUUID, MCPProxyUUID: proxyUUID})
		endpointByKey[key{proxyUUID, envUUID}] = endpointUUID
		configByEndpoint[endpointUUID] = models.MCPEndpointConfig{ToolScopeBindings: b.bindings}
	}

	configRepo := &repomocks.AgentConfigurationRepositoryMock{
		GetByAgentIDFunc: func(_ context.Context, _, _ string) (*models.AgentConfiguration, error) {
			return &models.AgentConfiguration{EnvMCPMappings: mappings}, nil
		},
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		GetEndpointEnvByProxyAndEnvFunc: func(_ context.Context, proxyUUID, envUUID uuid.UUID) (*models.MCPProxyEndpointEnvironment, error) {
			endpointUUID, ok := endpointByKey[key{proxyUUID, envUUID}]
			if !ok {
				return nil, gorm.ErrRecordNotFound
			}
			return &models.MCPProxyEndpointEnvironment{EndpointUUID: endpointUUID}, nil
		},
		GetEndpointFunc: func(_ context.Context, endpointUUID uuid.UUID) (*models.MCPProxyEndpoint, error) {
			cfg, ok := configByEndpoint[endpointUUID]
			if !ok {
				return nil, gorm.ErrRecordNotFound
			}
			return &models.MCPProxyEndpoint{UUID: endpointUUID, Configuration: cfg}, nil
		},
	}
	return configRepo, endpointRepo
}

func TestResolveAgentIdentityScopes_NoAgentConfiguration_ReturnsEmpty(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyEndpointRepo(), &clientmocks.OpenChoreoClientMock{}, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	assert.Empty(t, scopes)
}

func TestResolveAgentIdentityScopes_SingleProxySingleTool_ReturnsItsScopes(t *testing.T) {
	envUUID := "11111111-1111-1111-1111-111111111111"
	configRepo, endpointRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID: envUUID,
		bindings: []models.MCPToolScopeBinding{
			{Tool: "search_tickets", Scopes: []string{"amp:mcp:tickets:read"}},
		},
	})
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, endpointRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	assert.Equal(t, []string{"amp:mcp:tickets:read"}, scopes)
}

func TestResolveAgentIdentityScopes_MultipleProxiesAndTools_DedupsUnion(t *testing.T) {
	envUUID := "22222222-2222-2222-2222-222222222222"
	configRepo, endpointRepo := mcpBoundAgentConfigRepo(
		mcpProxyBinding{
			envUUID: envUUID,
			bindings: []models.MCPToolScopeBinding{
				{Tool: "search_tickets", Scopes: []string{"amp:mcp:tickets:read", "amp:mcp:tickets:write"}},
			},
		},
		mcpProxyBinding{
			envUUID: envUUID,
			bindings: []models.MCPToolScopeBinding{
				{Tool: "create_incident", Scopes: []string{"amp:mcp:incidents:write"}},
				// Same scope as the other proxy's tool — must appear only once in the result.
				{Tool: "list_incidents", Scopes: []string{"amp:mcp:tickets:read"}},
			},
		},
	)
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, endpointRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	assert.Equal(t, []string{"amp:mcp:incidents:write", "amp:mcp:tickets:read", "amp:mcp:tickets:write"}, scopes,
		"must be the deduplicated, sorted union across every bound proxy's tools")
}

func TestResolveAgentIdentityScopes_MappingForDifferentEnvironment_Ignored(t *testing.T) {
	boundEnvUUID := "33333333-3333-3333-3333-333333333333"
	otherEnvUUID := "44444444-4444-4444-4444-444444444444"
	// The proxy has an endpoint deployed to otherEnvUUID, but the agent's own
	// mapping is bound to boundEnvUUID (matches the binding's environment) —
	// simulating a proxy that isn't (or is no longer) configured for the
	// environment this agent is actually deployed to. The join lookup for
	// (proxy, boundEnvUUID) must find nothing, even though the proxy DOES
	// have an endpoint deployed to some other environment.
	proxyUUID := uuid.New()
	configRepo := &repomocks.AgentConfigurationRepositoryMock{
		GetByAgentIDFunc: func(_ context.Context, _, _ string) (*models.AgentConfiguration, error) {
			return &models.AgentConfiguration{EnvMCPMappings: []models.EnvAgentMCPMapping{
				{EnvironmentUUID: uuid.MustParse(boundEnvUUID), MCPProxyUUID: proxyUUID},
			}}, nil
		},
	}
	endpointRepo := &repomocks.MCPProxyEndpointRepositoryMock{
		GetEndpointEnvByProxyAndEnvFunc: func(_ context.Context, gotProxy, gotEnv uuid.UUID) (*models.MCPProxyEndpointEnvironment, error) {
			if gotProxy == proxyUUID && gotEnv.String() == otherEnvUUID {
				return &models.MCPProxyEndpointEnvironment{EndpointUUID: uuid.New()}, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: boundEnvUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, endpointRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	assert.Empty(t, scopes)
}

func TestResolveAgentIdentityScopes_AgentConfigLoadError_FailsClosed(t *testing.T) {
	failingRepo := &repomocks.AgentConfigurationRepositoryMock{
		GetByAgentIDFunc: func(_ context.Context, _, _ string) (*models.AgentConfiguration, error) {
			return nil, errors.New("db unavailable")
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		failingRepo, noMCPProxyEndpointRepo(), &clientmocks.OpenChoreoClientMock{}, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	assert.Empty(t, scopes, "a Thunder/DB lookup failure must fail closed to no scopes, never a stale or placeholder set")
}

func TestResolveAgentIdentityScopes_EnvironmentResolveError_FailsClosed(t *testing.T) {
	envUUID := "55555555-5555-5555-5555-555555555555"
	configRepo, endpointRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID: envUUID,
		bindings: []models.MCPToolScopeBinding{
			{Tool: "search_tickets", Scopes: []string{"amp:mcp:tickets:read"}},
		},
	})
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return nil, errors.New("environment resolution failed")
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, endpointRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	assert.Empty(t, scopes)
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_UpdatesExistingSecretReference(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	updated := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			updated = true
			assert.Equal(t, testIdentityKVPath, req.KVPath)
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		// CreateSecretReferenceFunc deliberately nil — a Create call would panic the test.
	}
	svc := newTestIdentityInjectionService(repo, oc)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	assert.Len(t, envVars, 4)
	assert.True(t, updated)
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_CreateConflictFallsBackToUpdate(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	updated := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, _ string) (*client.SecretReferenceInfo, error) {
			return nil, utils.ErrNotFound
		},
		CreateSecretReferenceFunc: func(_ context.Context, _ string, _ client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return nil, utils.ErrConflict
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, _ client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			updated = true
			return &client.SecretReferenceInfo{}, nil
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	assert.Len(t, envVars, 4)
	assert.True(t, updated, "create conflict (concurrent creator) must fall back to update, not fail")
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_SkipStates(t *testing.T) {
	pending := completedInternalBinding()
	pending.Status = models.AgentThunderStatusPending

	failed := completedInternalBinding()
	failed.Status = models.AgentThunderStatusFailed

	external := completedInternalBinding()
	external.ProvisioningType = models.AgentProvisioningTypeExternal

	revoked := completedInternalBinding()
	revoked.SecretRefPath = ""

	noClientID := completedInternalBinding()
	noClientID.ThunderClientID = ""

	cases := []struct {
		name    string
		binding *models.AgentThunderClient
		repoErr error
	}{
		{name: "no binding", binding: nil, repoErr: repositories.ErrAgentThunderClientNotFound},
		{name: "pending binding", binding: pending},
		{name: "failed binding", binding: failed},
		{name: "external agent", binding: external},
		{name: "revoked credential", binding: revoked},
		{name: "missing client id", binding: noClientID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := identityRepoReturning(tc.binding, tc.repoErr)
			// All OpenChoreo funcs nil: any CR call would panic — proving
			// skip states never touch OpenChoreo.
			svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

			envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
			require.NoError(t, err)
			assert.Nil(t, envVars)
		})
	}
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_RepoErrorPropagates(t *testing.T) {
	repoErr := errors.New("db down")
	repo := identityRepoReturning(nil, repoErr)
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, repoErr, "a real repo error must surface, never be masked as 'nothing to inject'")
	assert.Nil(t, envVars)
}

func TestAgentIdentityInjection_EnvVarsForEnvironment_SecretReferenceCheckErrorPropagates(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	ocErr := errors.New("openchoreo unavailable")
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, _ string) (*client.SecretReferenceInfo, error) {
			return nil, ocErr
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, ocErr)
	assert.Nil(t, envVars)
}

func TestAgentIdentityInjection_InjectForEnvironment_PushesVarsIntoReleaseBinding(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	var injectedEnv string
	var injectedVars []client.EnvVar
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, envName string, envVars []client.EnvVar) error {
			injectedEnv = envName
			injectedVars = envVars
			return nil
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, testIdentityEnv, injectedEnv)
	assert.Len(t, injectedVars, 4)
}

func TestAgentIdentityInjection_InjectForEnvironment_NothingToInject_NoWorkloadCalls(t *testing.T) {
	repo := identityRepoReturning(nil, repositories.ErrAgentThunderClientNotFound)
	// UpdateReleaseBindingEnvVarsFunc nil — a call would panic.
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	assert.NoError(t, svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_InjectForEnvironment_WorkloadUpdateErrorPropagates(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	updateErr := errors.New("binding update failed")
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
			return updateErr
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	assert.ErrorIs(t, err, updateErr)
}

// inSyncIdentityEnvVars returns the live env vars a fully-injected workload
// would report back for the no-MCP completedInternalBinding(): all four AgentID
// keys present, with an empty scope list (no MCP bindings). Used as the
// "already in sync" baseline for the reconcile tests.
func inSyncIdentityEnvVars() []models.EnvVars {
	refName := AgentIdentitySecretRefName(testIdentityAgent, testIdentityEnv)
	return []models.EnvVars{
		{Key: "AMP_OTEL_ENDPOINT", Value: "http://otel"}, // unrelated base var, must be ignored
		{Key: client.EnvVarAgentIdentityClientID, Value: "client-abc"},
		{Key: client.EnvVarAgentIdentityClientSecret, IsSensitive: true, SecretRef: refName, SecretKey: thundersvc.AgentSecretKeyClientSecret},
		{Key: client.EnvVarAgentIdentityTokenEndpoint, Value: thundersvc.ThunderTokenURL(ThunderOrgNamespace(), testIdentityEnv)},
		{Key: client.EnvVarAgentIdentityScopes, Value: ""},
	}
}

func TestAgentIdentityInjection_ReconcileForEnvironment_InSync_DoesNotWrite(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		GetComponentConfigurationsFunc: func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
			return inSyncIdentityEnvVars(), nil
		},
		// UpdateReleaseBindingEnvVarsFunc left nil — a call would panic, proving
		// an already-in-sync workload is never re-written (no needless pod roll).
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_ReconcileForEnvironment_MissingVars_Injects(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	injectedVars := 0
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		GetComponentConfigurationsFunc: func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
			// Workload just came up from a first build; only base vars present, no identity vars.
			return []models.EnvVars{{Key: "AMP_OTEL_ENDPOINT", Value: "http://otel"}}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
			injectedVars = len(envVars)
			return nil
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, 4, injectedVars, "a workload missing the identity vars must be injected with the full set")
}

func TestAgentIdentityInjection_ReconcileForEnvironment_ScopeDrift_Reinjects(t *testing.T) {
	envUUID := "44444444-4444-4444-4444-444444444444"
	configRepo, endpointRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID: envUUID,
		bindings: []models.MCPToolScopeBinding{
			{Tool: "search_tickets", Scopes: []string{"amp:mcp:tickets:read"}},
		},
	})
	var injectedScopes string
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID}, nil
		},
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		GetComponentConfigurationsFunc: func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
			// All four keys present, but the live scopes are stale (empty) vs the
			// now-desired "amp:mcp:tickets:read".
			refName := AgentIdentitySecretRefName(testIdentityAgent, testIdentityEnv)
			return []models.EnvVars{
				{Key: client.EnvVarAgentIdentityClientID, Value: "client-abc"},
				{Key: client.EnvVarAgentIdentityClientSecret, IsSensitive: true, SecretRef: refName, SecretKey: thundersvc.AgentSecretKeyClientSecret},
				{Key: client.EnvVarAgentIdentityTokenEndpoint, Value: thundersvc.ThunderTokenURL(ThunderOrgNamespace(), testIdentityEnv)},
				{Key: client.EnvVarAgentIdentityScopes, Value: ""},
			}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
			for _, ev := range envVars {
				if ev.Key == client.EnvVarAgentIdentityScopes {
					injectedScopes = ev.Value
				}
			}
			return nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, endpointRepo, oc, "1h", discardLogger())

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, "amp:mcp:tickets:read", injectedScopes, "a drifted scope list must be re-injected with the current scopes")
}

func TestAgentIdentityInjection_ReconcileForEnvironment_NothingToInject_NoReadOrWrite(t *testing.T) {
	repo := identityRepoReturning(nil, repositories.ErrAgentThunderClientNotFound)
	// GetComponentConfigurationsFunc / UpdateReleaseBindingEnvVarsFunc left nil —
	// a call would panic, proving an uninjectable binding short-circuits before
	// touching the workload at all.
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_ReconcileForEnvironment_ConfigReadError_Propagates(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		GetComponentConfigurationsFunc: func(_ context.Context, _, _, _, _ string) ([]models.EnvVars, error) {
			return nil, errors.New("openchoreo unavailable")
		},
		// UpdateReleaseBindingEnvVarsFunc left nil — must not write when it can't
		// determine the current state.
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	assert.Error(t, err, "an unreadable current state must not silently proceed to a blind write")
}

func TestAgentIdentityInjection_RefreshAfterRotation_StampsAnnotationAndRollsPod(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	fixedNow := time.Date(2026, 7, 8, 10, 30, 0, 0, time.UTC)
	var updateReq client.CreateSecretReferenceRequest
	rolled := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			updateReq = req
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
			rolled = true
			return nil
		},
	}

	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyEndpointRepo(), oc, "1h", discardLogger())
	impl, ok := svc.(*agentIdentityInjectionService)
	require.True(t, ok)
	impl.now = func() time.Time { return fixedNow }

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	require.NotNil(t, updateReq.TemplateAnnotations)
	assert.Equal(t, fixedNow.Format(secretRotatedAtFormat), updateReq.TemplateAnnotations[secretRotatedAtAnnotation],
		"rotation must stamp a fresh annotation so the controller re-syncs the Secret immediately")
	assert.True(t, rolled, "rotation must roll the pod so it starts with the refreshed Secret")
}

func TestAgentIdentityInjection_RefreshAfterRotation_NoBinding_NoOp(t *testing.T) {
	repo := identityRepoReturning(nil, repositories.ErrAgentThunderClientNotFound)
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	assert.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
}

func TestAgentIdentityInjection_RemoveForEnvironment_RemovesVarsAndSecretReference(t *testing.T) {
	// Post-revoke state: still internal + completed, but no stored secret.
	binding := completedInternalBinding()
	binding.SecretRefPath = ""
	repo := identityRepoReturning(binding, nil)

	var removedKeys []string
	deletedRef := ""
	oc := &clientmocks.OpenChoreoClientMock{
		RemoveReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, envName string, keys []string) error {
			assert.Equal(t, testIdentityEnv, envName)
			removedKeys = keys
			return nil
		},
		DeleteSecretReferenceFunc: func(_ context.Context, _, refName string) error {
			deletedRef = refName
			return nil
		},
		// RemoveWorkloadEnvVarsFunc nil — includeWorkloadLevel=false must not touch the workload.
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.RemoveForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv, false))

	expectedKeys := make([]string, 0, 4)
	for k := range AgentIdentityEnvVarKeys() {
		expectedKeys = append(expectedKeys, k)
	}
	assert.ElementsMatch(t, expectedKeys, removedKeys)
	assert.Equal(t, AgentIdentitySecretRefName(testIdentityAgent, testIdentityEnv), deletedRef)
}

func TestAgentIdentityInjection_RemoveForEnvironment_IncludeWorkloadLevel(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	workloadRemoved := false
	oc := &clientmocks.OpenChoreoClientMock{
		RemoveReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []string) error { return nil },
		RemoveWorkloadEnvVarsFunc: func(_ context.Context, _, _ string, keys []string) error {
			workloadRemoved = true
			assert.Len(t, keys, 4)
			return nil
		},
		DeleteSecretReferenceFunc: func(_ context.Context, _, _ string) error { return nil },
	}
	svc := newTestIdentityInjectionService(repo, oc)

	require.NoError(t, svc.RemoveForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv, true))
	assert.True(t, workloadRemoved)
}

func TestAgentIdentityInjection_RemoveForEnvironment_ExternalAgent_NoOp(t *testing.T) {
	binding := completedInternalBinding()
	binding.ProvisioningType = models.AgentProvisioningTypeExternal
	repo := identityRepoReturning(binding, nil)
	// All OpenChoreo funcs nil — any call would panic.
	svc := newTestIdentityInjectionService(repo, &clientmocks.OpenChoreoClientMock{})

	assert.NoError(t, svc.RemoveForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv, true))
}

func TestAgentIdentityInjection_RemoveForEnvironment_SecretRefNotFound_Tolerated(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	oc := &clientmocks.OpenChoreoClientMock{
		RemoveReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []string) error { return nil },
		DeleteSecretReferenceFunc: func(_ context.Context, _, _ string) error {
			return utils.ErrNotFound
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	assert.NoError(t, svc.RemoveForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv, false))
}

func TestAgentIdentityInjection_CleanupForEnvironment_DeletesSecretReference(t *testing.T) {
	deletedRef := ""
	oc := &clientmocks.OpenChoreoClientMock{
		DeleteSecretReferenceFunc: func(_ context.Context, namespace, refName string) error {
			assert.Equal(t, testIdentityOrg, namespace)
			deletedRef = refName
			return nil
		},
	}
	svc := newTestIdentityInjectionService(&repomocks.AgentThunderClientRepositoryMock{}, oc)

	require.NoError(t, svc.CleanupForEnvironment(context.Background(), testIdentityOrg, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, AgentIdentitySecretRefName(testIdentityAgent, testIdentityEnv), deletedRef)
}

func TestAgentIdentitySecretRefName_SanitizesAndTruncates(t *testing.T) {
	assert.Equal(t, "my-agent-staging-agent-identity", AgentIdentitySecretRefName("my-agent", "staging"))
	// Uppercase and invalid runes are sanitized.
	assert.Equal(t, "my-agent-stag-ing-agent-identity", AgentIdentitySecretRefName("My_Agent", "Stag.ing"))

	long1 := AgentIdentitySecretRefName(strings.Repeat("a", 60), "env-a")
	long2 := AgentIdentitySecretRefName(strings.Repeat("a", 60), "env-b")
	assert.LessOrEqual(t, len(long1), 63, "must respect the Kubernetes name length limit")
	assert.NotEqual(t, "-", long1[len(long1)-1:], "must not end with a trailing hyphen after truncation")
	assert.NotEqual(t, long1, long2, "different env names with same long agent name prefix must not collide")
}

func TestAgentIdentitySecretRefName_LongNamesDoNotCollideAfterTruncation(t *testing.T) {
	// Two distinct agent names that are identical for the first 60 characters
	// (only the tail differs) — plain truncation at 63 chars would produce
	// the exact same prefix for both, silently colliding onto one
	// SecretReference name unless the hash suffix disambiguates them.
	agentA := strings.Repeat("a", 60) + "-team-alpha"
	agentB := strings.Repeat("a", 60) + "-team-beta"
	env := "production"

	nameA := AgentIdentitySecretRefName(agentA, env)
	nameB := AgentIdentitySecretRefName(agentB, env)

	assert.LessOrEqual(t, len(nameA), 63)
	assert.LessOrEqual(t, len(nameB), 63)
	assert.NotEqual(t, nameA, nameB,
		"two different agent names sharing a 63-char prefix must not collide onto the same SecretReference name")
	assert.True(t, strings.HasSuffix(nameA, "-"+agentIdentitySecretRefSuffix))
	assert.True(t, strings.HasSuffix(nameB, "-"+agentIdentitySecretRefSuffix))
}

func TestAgentIdentitySecretRefName_TruncationIsDeterministic(t *testing.T) {
	longAgent := strings.Repeat("x", 80)
	first := AgentIdentitySecretRefName(longAgent, "dev")
	second := AgentIdentitySecretRefName(longAgent, "dev")
	assert.Equal(t, first, second, "the same (agent, env) pair must always produce the same name")
}

func TestAgentIdentityInjection_RefreshAfterRotation_TwoRotationsInSameSecondProduceDistinctAnnotations(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	var annotations []string
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			annotations = append(annotations, req.TemplateAnnotations[secretRotatedAtAnnotation])
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error { return nil },
	}

	// Same wall-clock SECOND for both rotations — only nanoseconds differ,
	// exactly the scenario time.RFC3339 (second precision) would collapse
	// into an identical annotation value.
	sameSecond := time.Date(2026, 7, 8, 10, 30, 0, 0, time.UTC)
	callNum := 0
	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyEndpointRepo(), oc, "1h", discardLogger())
	impl, ok := svc.(*agentIdentityInjectionService)
	require.True(t, ok)
	impl.now = func() time.Time {
		callNum++
		return sameSecond.Add(time.Duration(callNum) * time.Nanosecond)
	}

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))

	require.Len(t, annotations, 2)
	assert.NotEqual(t, annotations[0], annotations[1],
		"two rotations within the same wall-clock second must still produce distinct annotation values, "+
			"otherwise the second rotation's CR update is a no-op spec-wise and the controller never re-syncs the new secret")
}

func TestAgentIdentityInjection_InjectForEnvironment_RetriesOnTransientConflictThenSucceeds(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	attempts := 0
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
			attempts++
			if attempts < 2 {
				return utils.ErrConflict
			}
			return nil
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err, "a transient conflict on the first attempt must be retried, not surfaced as a failure")
	assert.Equal(t, 2, attempts)
}

func TestAgentIdentityInjection_InjectForEnvironment_GivesUpAfterRetriesExhausted(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	attempts := 0
	persistentErr := errors.New("release binding permanently conflicted")
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
			attempts++
			return persistentErr
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, persistentErr)
	assert.Equal(t, releaseBindingUpdateRetries, attempts, "must give up after the bounded retry budget, not retry forever")
}
