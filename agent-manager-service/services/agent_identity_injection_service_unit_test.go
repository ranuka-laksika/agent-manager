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
)

// testIdentityKVPath is the deterministic KV path agentIdentitySecretLocation
// computes for the fixed (org, project, agent, env) tuple above — computed
// via the real function, not hardcoded, so this fixture can't silently drift
// out of sync with what ensureSecretReference actually derives.
func testIdentityKVPath() string {
	kvPath, err := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv).KVPath()
	if err != nil {
		panic(err) // fixed, non-empty test constants — must never fail
	}
	return kvPath
}

// testIdentitySecretRefName is the deterministic SecretReference CR name
// agentIdentitySecretLocation computes for the fixed (org, project, agent,
// env) tuple above.
func testIdentitySecretRefName() string {
	return agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv).SecretRefName()
}

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
		SecretRefPath:    testIdentityKVPath(),
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

// noMCPProxyScopeRepo returns an MCPProxyScopeRepository mock with every
// method left unset — any unexpected call panics (moq's default), which is
// exactly right for tests whose scope resolution short-circuits before ever
// needing it (e.g. no agent configuration).
func noMCPProxyScopeRepo() *repomocks.MCPProxyScopeRepositoryMock {
	return &repomocks.MCPProxyScopeRepositoryMock{}
}

func newTestIdentityInjectionService(
	repo *repomocks.AgentThunderClientRepositoryMock,
	oc *clientmocks.OpenChoreoClientMock,
) AgentIdentityInjectionService {
	return NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), oc, "1h", discardLogger())
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

	expectedRefName := testIdentitySecretRefName()
	assert.Equal(t, expectedRefName, createdReq.Name)
	assert.Equal(t, testIdentityKVPath(), createdReq.KVPath, "SecretReference must point at the EXISTING stored secret — no secret duplication")
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
// bound to an environment, carrying the given scope actions — the real chain
// resolveAgentIdentityScopes now walks (proxy -> its own MCPProxyScope rows),
// not a per-environment tool binding.
type mcpProxyBinding struct {
	envUUID      string
	proxyHandle  string
	scopeActions []string
}

// mcpBoundAgentConfigRepo returns an AgentConfigurationRepository mock whose
// GetByAgentID returns a config with one EnvAgentMCPMapping (preloaded
// MCPProxy included, matching GetByAgentID's real preload chain) per given
// binding, plus the MCPProxyScopeRepository mock that returns each proxy's
// scope rows — together, what resolveAgentIdentityScopes needs to aggregate
// scope strings.
func mcpBoundAgentConfigRepo(bindings ...mcpProxyBinding) (*repomocks.AgentConfigurationRepositoryMock, *repomocks.MCPProxyScopeRepositoryMock) {
	mappings := make([]models.EnvAgentMCPMapping, 0, len(bindings))
	scopesByProxy := map[uuid.UUID][]models.MCPProxyScope{}

	for _, b := range bindings {
		proxyUUID := uuid.New()
		envUUID := uuid.MustParse(b.envUUID)
		proxy := &models.MCPProxy{UUID: proxyUUID, Artifact: &models.Artifact{Handle: b.proxyHandle}}
		mappings = append(mappings, models.EnvAgentMCPMapping{EnvironmentUUID: envUUID, MCPProxyUUID: proxyUUID, MCPProxy: proxy})
		scopes := make([]models.MCPProxyScope, 0, len(b.scopeActions))
		for _, action := range b.scopeActions {
			scopes = append(scopes, models.MCPProxyScope{MCPProxyUUID: proxyUUID, Action: action})
		}
		scopesByProxy[proxyUUID] = scopes
	}

	configRepo := &repomocks.AgentConfigurationRepositoryMock{
		GetByAgentIDFunc: func(_ context.Context, _, _ string) (*models.AgentConfiguration, error) {
			return &models.AgentConfiguration{EnvMCPMappings: mappings}, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyUUIDsFunc: func(_ context.Context, proxyUUIDs []uuid.UUID) ([]models.MCPProxyScope, error) {
			out := make([]models.MCPProxyScope, 0, len(proxyUUIDs))
			for _, id := range proxyUUIDs {
				out = append(out, scopesByProxy[id]...)
			}
			return out, nil
		},
	}
	return configRepo, scopeRepo
}

func TestResolveAgentIdentityScopes_NoAgentConfiguration_ReturnsEmpty(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)
	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), &clientmocks.OpenChoreoClientMock{}, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Empty(t, scopes)
}

func TestResolveAgentIdentityScopes_SingleProxySingleTool_ReturnsItsScopes(t *testing.T) {
	envUUID := "11111111-1111-1111-1111-111111111111"
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID:      envUUID,
		proxyHandle:  "tickets",
		scopeActions: []string{"read"},
	})
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Equal(t, []string{"tickets:read"}, scopes)
}

func TestResolveAgentIdentityScopes_MultipleProxies_ReturnsSortedUnion(t *testing.T) {
	envUUID := "22222222-2222-2222-2222-222222222222"
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(
		mcpProxyBinding{
			envUUID:      envUUID,
			proxyHandle:  "tickets",
			scopeActions: []string{"read", "write"},
		},
		mcpProxyBinding{
			envUUID:      envUUID,
			proxyHandle:  "incidents",
			scopeActions: []string{"write"},
		},
	)
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Equal(t, []string{"incidents:write", "tickets:read", "tickets:write"}, scopes,
		"must be the sorted union of every bound proxy's own scopes")
}

func TestResolveAgentIdentityScopes_MappingForDifferentEnvironment_Ignored(t *testing.T) {
	boundEnvUUID := "33333333-3333-3333-3333-333333333333"
	otherEnvUUID := "44444444-4444-4444-4444-444444444444"
	// The mapping is bound to otherEnvUUID, but the binding's own environment
	// is boundEnvUUID — simulating a proxy configured for a different
	// environment than the one this agent is actually deployed to. The
	// environment-UUID filter must skip this mapping entirely, so its scopes
	// are never even looked up.
	proxyUUID := uuid.New()
	configRepo := &repomocks.AgentConfigurationRepositoryMock{
		GetByAgentIDFunc: func(_ context.Context, _, _ string) (*models.AgentConfiguration, error) {
			return &models.AgentConfiguration{EnvMCPMappings: []models.EnvAgentMCPMapping{
				{
					EnvironmentUUID: uuid.MustParse(otherEnvUUID),
					MCPProxyUUID:    proxyUUID,
					MCPProxy:        &models.MCPProxy{UUID: proxyUUID, Artifact: &models.Artifact{Handle: "tickets"}},
				},
			}}, nil
		},
	}
	scopeRepo := &repomocks.MCPProxyScopeRepositoryMock{
		ListByProxyUUIDsFunc: func(context.Context, []uuid.UUID) ([]models.MCPProxyScope, error) {
			t.Fatal("must not look up scopes for a mapping bound to a different environment")
			return nil, nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: boundEnvUUID}, nil
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.NoError(t, err)
	assert.Empty(t, scopes)
}

// TestResolveAgentIdentityScopes_AgentConfigLoadError_PropagatesError guards
// against silently falling back to an empty scope list on a transient DB
// blip: every caller of this service already aborts on an error (deploy/
// promote/config-update all log "...to prevent credential loss" and stop),
// and ReconcileForEnvironment's no-needless-rollout guarantee depends on a
// trustworthy desired scope list — an empty list on a blip would look like a
// real scope change and cause a spurious rollout, then a second one once the
// blip cleared.
func TestResolveAgentIdentityScopes_AgentConfigLoadError_PropagatesError(t *testing.T) {
	failingRepo := &repomocks.AgentConfigurationRepositoryMock{
		GetByAgentIDFunc: func(_ context.Context, _, _ string) (*models.AgentConfiguration, error) {
			return nil, errors.New("db unavailable")
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		failingRepo, noMCPProxyScopeRepo(), &clientmocks.OpenChoreoClientMock{}, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.Error(t, err, "a DB lookup failure must propagate, not silently fail closed to an empty scope list")
	assert.Empty(t, scopes)
}

func TestResolveAgentIdentityScopes_EnvironmentResolveError_PropagatesError(t *testing.T) {
	envUUID := "55555555-5555-5555-5555-555555555555"
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID:      envUUID,
		proxyHandle:  "tickets",
		scopeActions: []string{"read"},
	})
	oc := &clientmocks.OpenChoreoClientMock{
		GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
			return nil, errors.New("environment resolution failed")
		},
	}
	svc := NewAgentIdentityInjectionService(identityRepoReturning(completedInternalBinding(), nil),
		configRepo, scopeRepo, oc, "1h", discardLogger())
	impl := svc.(*agentIdentityInjectionService)

	scopes, err := impl.resolveAgentIdentityScopes(context.Background(), completedInternalBinding())
	require.Error(t, err, "an OpenChoreo environment lookup failure must propagate, not silently fail closed to an empty scope list")
	assert.Empty(t, scopes)
}

// TestAgentIdentityInjection_EnvVarsForEnvironment_UpdatesExistingSecretReference
// covers an existing CR whose data sources don't yet match the desired KV
// path/keys (GetSecretReferenceFunc here returns no Data at all) — a genuine
// drift that must still be corrected.
func TestAgentIdentityInjection_EnvVarsForEnvironment_UpdatesExistingSecretReference(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	updated := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			updated = true
			assert.Equal(t, testIdentityKVPath(), req.KVPath)
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

// TestAgentIdentityInjection_EnvVarsForEnvironment_ExistingSecretReferenceAlreadyCorrect_SkipsUpdate
// guards against needless writes: an existing CR whose data sources already
// point at the exact desired KV path/keys must not be rewritten on every
// single call (one per deploy/promote/config-update, and one per reconciler
// tick per recently-completed binding) — both for the needless K8s API
// write, and because rewriting with nil TemplateAnnotations would otherwise
// silently clobber whatever annotation a prior rotation set.
func TestAgentIdentityInjection_EnvVarsForEnvironment_ExistingSecretReferenceAlreadyCorrect_SkipsUpdate(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{
				Name: refName,
				Data: []client.SecretDataSourceInfo{
					{SecretKey: thundersvc.AgentSecretKeyClientSecret, RemoteRef: client.RemoteRefInfo{Key: testIdentityKVPath()}},
				},
			}, nil
		},
		UpdateSecretReferenceFunc: func(context.Context, string, string, client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			t.Fatal("must not update a SecretReference that already points at the desired KV path and keys")
			return nil, nil //nolint:nilnil // unreachable — t.Fatal above halts the test
		},
		// CreateSecretReferenceFunc deliberately nil — a Create call would panic the test.
	}
	svc := newTestIdentityInjectionService(repo, oc)

	envVars, err := svc.EnvVarsForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err)
	assert.Len(t, envVars, 4)
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
	refName := testIdentitySecretRefName()
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
	configRepo, scopeRepo := mcpBoundAgentConfigRepo(mcpProxyBinding{
		envUUID:      envUUID,
		proxyHandle:  "tickets",
		scopeActions: []string{"read"},
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
			// now-desired "tickets:read".
			refName := testIdentitySecretRefName()
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
		configRepo, scopeRepo, oc, "1h", discardLogger())

	require.NoError(t, svc.ReconcileForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.Equal(t, "tickets:read", injectedScopes, "a drifted scope list must be re-injected with the current scopes")
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

	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), oc, "1h", discardLogger())
	impl, ok := svc.(*agentIdentityInjectionService)
	require.True(t, ok)
	impl.now = func() time.Time { return fixedNow }

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	require.NotNil(t, updateReq.TemplateAnnotations)
	assert.Equal(t, fixedNow.Format(secretRotatedAtFormat), updateReq.TemplateAnnotations[secretRotatedAtAnnotation],
		"rotation must stamp a fresh annotation so the controller re-syncs the Secret immediately")
	assert.True(t, rolled, "rotation must roll the pod so it starts with the refreshed Secret")
}

// TestAgentIdentityInjection_RefreshAfterRotation_AlwaysUpdatesEvenWhenDataAlreadyMatches
// guards the other half of the "skip when the CR already points at the
// desired KV path/keys" optimization: it must never apply to the rotation
// path, since rotation's whole point is to stamp a FRESH annotation value on
// every single call (by design, never "unchanged") to force the controller
// to re-sync immediately.
func TestAgentIdentityInjection_RefreshAfterRotation_AlwaysUpdatesEvenWhenDataAlreadyMatches(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	updated := false
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{
				Name: refName,
				Data: []client.SecretDataSourceInfo{
					{SecretKey: thundersvc.AgentSecretKeyClientSecret, RemoteRef: client.RemoteRefInfo{Key: testIdentityKVPath()}},
				},
			}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			updated = true
			require.NotEmpty(t, req.TemplateAnnotations, "rotation must always carry a fresh annotation")
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(context.Context, string, string, string, string, []client.EnvVar) error { return nil },
	}
	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), oc, "1h", discardLogger())

	require.NoError(t, svc.RefreshAfterRotation(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv))
	assert.True(t, updated, "rotation must always update the SecretReference, even when its data sources already match")
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
	assert.Equal(t, testIdentitySecretRefName(), deletedRef)
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
	assert.Equal(t, testIdentitySecretRefName(), deletedRef)
}

// TestAgentIdentitySecretLocation_EntityNameIsAgentScoped guards the specific
// property agentIdentitySecretLocation must hold: EntityName includes the
// agent name, not just a fixed "agent-identity" marker — secretmanagersvc's
// SecretRefName only derives the SecretReference CR name from EntityName (+
// EnvironmentName), so two different agents in the same environment would
// collide onto the identical CR name (one agent's credential silently
// overwriting another's) if EntityName weren't agent-scoped. Collision
// avoidance for very long names beyond that is secretmanagersvc's own
// concern (SecretLocation.SecretRefName), not re-tested here.
func TestAgentIdentitySecretLocation_EntityNameIsAgentScoped(t *testing.T) {
	locA := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, "agent-a", testIdentityEnv)
	locB := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, "agent-b", testIdentityEnv)

	assert.NotEqual(t, locA.SecretRefName(), locB.SecretRefName(),
		"two different agents in the same environment must never derive the same SecretReference name")
	assert.Contains(t, locA.EntityName, "agent-a")
}

// TestAgentIdentitySecretLocation_IsDeterministic guards the property both
// agentThunderProvisioningService (storing) and agentIdentityInjectionService
// (referencing) rely on: the same (org, project, agent, env) tuple must
// always compute the exact same KV path and CR name, with no stored or
// round-tripped state required to keep them in agreement.
func TestAgentIdentitySecretLocation_IsDeterministic(t *testing.T) {
	loc1 := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	loc2 := agentIdentitySecretLocation(testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)

	kvPath1, err := loc1.KVPath()
	require.NoError(t, err)
	kvPath2, err := loc2.KVPath()
	require.NoError(t, err)

	assert.Equal(t, kvPath1, kvPath2)
	assert.Equal(t, loc1.SecretRefName(), loc2.SecretRefName())
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
	svc := NewAgentIdentityInjectionService(repo, noMCPConfigRepo(), noMCPProxyScopeRepo(), oc, "1h", discardLogger())
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

// TestAgentIdentityInjection_InjectForEnvironment_RetriesOnInternalServerErrorConflict
// guards the realistic error path: OpenChoreo's UpdateReleaseBindingResp has no
// JSON409 field at all (see openchoreosvc/client.retryReleaseBindingUpdate's
// sibling comment), so a stale-resourceVersion conflict on this call can only
// ever surface as utils.ErrInternalServerError, never utils.ErrConflict. If the
// retry gate only matched ErrConflict, this exact scenario would never retry.
func TestAgentIdentityInjection_InjectForEnvironment_RetriesOnInternalServerErrorConflict(t *testing.T) {
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
				return utils.ErrInternalServerError
			}
			return nil
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.NoError(t, err, "a stale-resourceVersion conflict surfaced as a 500 (OpenChoreo's actual behavior for this call) must be retried")
	assert.Equal(t, 2, attempts)
}

func TestAgentIdentityInjection_InjectForEnvironment_GivesUpAfterRetriesExhausted(t *testing.T) {
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
			return utils.ErrConflict
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrConflict)
	assert.Equal(t, releaseBindingUpdateRetries, attempts, "must give up after the bounded retry budget, not retry forever")
}

func TestAgentIdentityInjection_InjectForEnvironment_DoesNotRetryPermanentError(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	attempts := 0
	permanentErr := errors.New("release binding validation failed")
	oc := &clientmocks.OpenChoreoClientMock{
		GetSecretReferenceFunc: func(_ context.Context, _, refName string) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: refName}, nil
		},
		UpdateSecretReferenceFunc: func(_ context.Context, _, _ string, req client.CreateSecretReferenceRequest) (*client.SecretReferenceInfo, error) {
			return &client.SecretReferenceInfo{Name: req.Name}, nil
		},
		UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, _ []client.EnvVar) error {
			attempts++
			return permanentErr
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(context.Background(), testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, permanentErr)
	assert.Equal(t, 1, attempts, "a non-conflict error is permanent and must not be retried")
}

func TestAgentIdentityInjection_InjectForEnvironment_StopsRetryingOnContextCancel(t *testing.T) {
	repo := identityRepoReturning(completedInternalBinding(), nil)

	ctx, cancel := context.WithCancel(context.Background())
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
			cancel() // simulate the caller's context being cancelled mid-retry
			return utils.ErrConflict
		},
	}
	svc := newTestIdentityInjectionService(repo, oc)

	err := svc.InjectForEnvironment(ctx, testIdentityOrg, testIdentityProject, testIdentityAgent, testIdentityEnv)
	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrConflict)
	assert.Equal(t, 1, attempts, "must stop retrying once the context is cancelled, not sleep out the full retry budget")
}
