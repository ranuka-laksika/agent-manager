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
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// stubAgentThunderProvisioning implements AgentThunderProvisioningService by
// embedding the (nil) interface and overriding only RegenerateFunc — any
// other method call panics on the nil embed, which is fine since tests using
// this stub never call them.
type stubAgentThunderProvisioning struct {
	AgentThunderProvisioningService
	RegenerateFunc func(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error)
	ClaimFunc      func(ctx context.Context, orgName, projectName, agentName, envName string) (string, string, string, error)
	RevokeFunc     func(ctx context.Context, orgName, projectName, agentName, envName string) (string, error)
}

func (s *stubAgentThunderProvisioning) RegenerateSecret(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error) {
	return s.RegenerateFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) ClaimSecret(ctx context.Context, orgName, projectName, agentName, envName string) (string, string, string, error) {
	return s.ClaimFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) RevokeSecret(ctx context.Context, orgName, projectName, agentName, envName string) (string, error) {
	return s.RevokeFunc(ctx, orgName, projectName, agentName, envName)
}

func TestValidateInstrumentationVersion_UsesCatalog(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validateInstrumentationVersion("0.2.1"); err != nil {
		t.Errorf("0.2.1 should be valid: %v", err)
	}
	err := s.validateInstrumentationVersion("9.9.9")
	if err == nil {
		t.Fatal("9.9.9 should be invalid")
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("error %q should mention 9.9.9", err)
	}
}

func TestValidatePythonInstrumentationPair(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
			{Version: "0.4.0", PythonVersions: []string{"3.12", "3.13"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validatePythonInstrumentationPair("3.11", "0.2.1"); err != nil {
		t.Errorf("3.11 + 0.2.1 should be valid: %v", err)
	}
	err := s.validatePythonInstrumentationPair("3.13", "0.2.1")
	if err == nil {
		t.Fatal("3.13 + 0.2.1 should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should mention both python and instrumentation versions", err)
	}
}

func TestValidateEffectivePair_FallsBackToDefault(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// nil requested version means "use platform default", which is 0.2.1.
	if err := s.validateEffectivePythonInstrumentationPair("3.11", nil); err != nil {
		t.Errorf("3.11 + default(0.2.1) should be valid: %v", err)
	}
	err := s.validateEffectivePythonInstrumentationPair("3.13", nil)
	if err == nil {
		t.Fatal("3.13 + default(0.2.1) should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should name the resolved default version, not just nil", err)
	}
}

func TestBuildpackPythonVersion_Normalises(t *testing.T) {
	mk := func(lang string, version *string) *spec.Build {
		b := spec.BuildpackBuildAsBuild(&spec.BuildpackBuild{
			Buildpack: spec.BuildpackConfig{
				Language:        lang,
				LanguageVersion: version,
			},
		})
		return &b
	}
	strPtr := func(s string) *string { return &s }

	cases := []struct {
		name string
		in   *spec.Build
		want string
	}{
		{"bare minor", mk("python", strPtr("3.11")), "3.11"},
		{"with patch", mk("python", strPtr("3.11.4")), "3.11"},
		{"with x", mk("python", strPtr("3.11.x")), "3.11"},
		{"leading whitespace", mk("python", strPtr("  3.11  ")), "3.11"},
		{"whitespace only", mk("python", strPtr("   ")), ""},
		{"empty", mk("python", strPtr("")), ""},
		{"capital P language", mk("Python", strPtr("3.11")), ""},
		{"non python language", mk("nodejs", strPtr("20")), ""},
		{"single component", mk("python", strPtr("3")), ""},
		{"nil version", mk("python", nil), ""},
		{"nil build", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildpackPythonVersion(tc.in)
			if got != tc.want {
				t.Errorf("buildpackPythonVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateEffectivePair_NoPythonIsNoOp(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// Empty python means the agent isn't a python-buildpack build.
	if err := s.validateEffectivePythonInstrumentationPair("", nil); err != nil {
		t.Errorf("empty python should be a no-op: %v", err)
	}
}

func TestNormalizePythonMinor(t *testing.T) {
	cases := map[string]string{
		"3.11":     "3.11",
		"3.11.4":   "3.11",
		"3.11.x":   "3.11",
		"  3.11  ": "3.11",
		"3":        "",
		"":         "",
		"   ":      "",
	}
	for in, want := range cases {
		if got := normalizePythonMinor(in); got != want {
			t.Errorf("normalizePythonMinor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveInstrumentationImageOverride(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
			{Version: "0.4.0", PythonVersions: []string{"3.12", "3.13"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	strPtr := func(s string) *string { return &s }
	s := &agentManagerService{logger: discardLogger()}

	t.Run("non-python echoes existing pin, no image", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(false, "3.11", strPtr("0.4.0"), strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want existing 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty for non-python", image)
		}
	})

	t.Run("request override validates and wins", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("0.2.1"), strPtr("0.4.0"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want requested 0.2.1", version)
		}
		if !strings.HasSuffix(image, "0.2.1-python3.11") {
			t.Errorf("image = %q, want suffix 0.2.1-python3.11", image)
		}
	})

	t.Run("unknown request version is rejected", func(t *testing.T) {
		_, _, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("9.9.9"), nil)
		if !errors.Is(err, utils.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("python-incompatible request version is rejected", func(t *testing.T) {
		// 0.4.0 supports 3.12/3.13, not 3.11.
		_, _, err := s.resolveInstrumentationImageOverride(true, "3.11", strPtr("0.4.0"), nil)
		if !errors.Is(err, utils.ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("no request preserves existing pin as image", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if !strings.HasSuffix(image, "0.2.1-python3.11") {
			t.Errorf("image = %q, want suffix 0.2.1-python3.11", image)
		}
	})

	t.Run("no request and no existing pin yields no override", func(t *testing.T) {
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.11", nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version != nil {
			t.Errorf("version = %v, want nil", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty", image)
		}
	})

	t.Run("existing pin incompatible with current python keeps version but skips image", func(t *testing.T) {
		// Pin 0.2.1 supports 3.10/3.11; the agent's Python is now 3.13. Building
		// the image would yield a nonexistent 0.2.1-python3.13 tag, so the
		// override is skipped (empty image) while the DB version is preserved.
		version, image, err := s.resolveInstrumentationImageOverride(true, "3.13", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty (incompatible pin, component default kept)", image)
		}
	})

	t.Run("existing pin with unparseable python keeps component default", func(t *testing.T) {
		// No request override + bad language version: don't fail the redeploy,
		// just skip the per-env image override.
		version, image, err := s.resolveInstrumentationImageOverride(true, "notaversion", nil, strPtr("0.2.1"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version == nil || *version != "0.2.1" {
			t.Errorf("version = %v, want preserved 0.2.1", version)
		}
		if image != "" {
			t.Errorf("image = %q, want empty (component default kept)", image)
		}
	})
}

func TestRegenerateAgentIdentitySecret_ExternalAgent_ReturnsSecret(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeExternal, "client-abc", "fresh-secret-xyz", nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, "dev", resp.EnvironmentName)
	assert.Equal(t, models.AgentProvisioningTypeExternal, resp.ProvisioningType)
	assert.Equal(t, "client-abc", resp.ClientID)
	assert.Equal(t, "fresh-secret-xyz", resp.ClientSecret,
		"an External agent must get its freshly regenerated secret back")
	assert.Equal(t, models.AgentRegenerateSecretStatus, resp.Status)
}

func TestRegenerateAgentIdentitySecret_InternalAgent_AlsoReturnsSecret(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeInternal, "client-def", "fresh-secret-internal", nil
		},
	}
	refreshed := 0
	injector := &agentIdentityInjectorStub{
		RefreshAfterRotationFunc: func(_ context.Context, orgName, _, _, envName string) error {
			refreshed++
			assert.Equal(t, "acme", orgName)
			assert.Equal(t, "dev", envName)
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, logger: discardLogger()}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeInternal, resp.ProvisioningType)
	assert.Equal(t, "fresh-secret-internal", resp.ClientSecret,
		"an Internal agent must ALSO get its freshly regenerated secret back — regenerate is not the "+
			"one-time-claim endpoint, withholding it here would just force a second call to see it")
	assert.Equal(t, models.AgentRegenerateSecretStatus, resp.Status)
	assert.Equal(t, 1, refreshed, "internal rotation must refresh the workload's injected credential")
}

func TestRegenerateAgentIdentitySecret_InternalAgent_RefreshFailureDoesNotFailRotation(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeInternal, "client-def", "fresh-secret-internal", nil
		},
	}
	injector := &agentIdentityInjectorStub{
		RefreshAfterRotationFunc: func(_ context.Context, _, _, _, _ string) error {
			return errors.New("workload refresh failed")
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, logger: discardLogger()}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err, "the rotation already happened in Thunder — a failed workload refresh must not fail the request")
	assert.Equal(t, "fresh-secret-internal", resp.ClientSecret)
}

func TestRevokeAgentIdentitySecret_LowestEnv_RemovesWorkloadLevelVars(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(_ context.Context, _, _, _, _ string) (string, error) { return "client-abc", nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
	}
	var gotIncludeWorkload *bool
	injector := &agentIdentityInjectorStub{
		RemoveForEnvironmentFunc: func(_ context.Context, _, _, _, envName string, includeWorkloadLevel bool) error {
			assert.Equal(t, "dev", envName)
			gotIncludeWorkload = &includeWorkloadLevel
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, ocClient: ocClient, logger: discardLogger()}

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, "client-abc", resp.ClientID)
	require.NotNil(t, gotIncludeWorkload)
	assert.True(t, *gotIncludeWorkload, "revoking the LOWEST environment's credential must also strip the shared workload-level vars")
}

func TestRevokeAgentIdentitySecret_NonLowestEnv_KeepsWorkloadLevelVars(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(_ context.Context, _, _, _, _ string) (string, error) { return "client-abc", nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
	}
	var gotIncludeWorkload *bool
	injector := &agentIdentityInjectorStub{
		RemoveForEnvironmentFunc: func(_ context.Context, _, _, _, envName string, includeWorkloadLevel bool) error {
			assert.Equal(t, "staging", envName)
			gotIncludeWorkload = &includeWorkloadLevel
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, ocClient: ocClient, logger: discardLogger()}

	_, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	require.NotNil(t, gotIncludeWorkload)
	assert.False(t, *gotIncludeWorkload, "revoking a NON-lowest environment must never strip the lowest environment's shared workload-level vars")
}

func TestRevokeAgentIdentitySecret_PipelineLookupFails_StillRevokesConservatively(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(_ context.Context, _, _, _, _ string) (string, error) { return "client-abc", nil },
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return nil, errors.New("pipeline unavailable")
		},
	}
	var gotIncludeWorkload *bool
	injector := &agentIdentityInjectorStub{
		RemoveForEnvironmentFunc: func(_ context.Context, _, _, _, _ string, includeWorkloadLevel bool) error {
			gotIncludeWorkload = &includeWorkloadLevel
			return nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, agentIdentityInjection: injector, ocClient: ocClient, logger: discardLogger()}

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err, "revoke already succeeded in Thunder — cleanup problems must not fail the request")
	assert.Equal(t, "client-abc", resp.ClientID)
	require.NotNil(t, gotIncludeWorkload)
	assert.False(t, *gotIncludeWorkload, "with an unknown pipeline, be conservative and leave workload-level vars alone")
}

func TestClaimAgentIdentitySecret_ExternalAgent_ReturnsSecret(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.ExternalAgent)}}, nil
		},
	}
	stub := &stubAgentThunderProvisioning{
		ClaimFunc: func(_ context.Context, _, _, _, _ string) (string, string, string, error) {
			return "agent-uuid", "client-abc", "claimed-secret-xyz", nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	resp, err := s.ClaimAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, "dev", resp.EnvironmentName)
	assert.Equal(t, "agent-uuid", resp.AgentID)
	assert.Equal(t, "client-abc", resp.ClientID)
	assert.Equal(t, "claimed-secret-xyz", resp.ClientSecret)
	assert.Equal(t, models.AgentClaimSecretStatus, resp.Status)
}

func TestClaimAgentIdentitySecret_InternalAgent_RejectedBeforeClaim(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
	}
	stub := &stubAgentThunderProvisioning{
		ClaimFunc: func(_ context.Context, _, _, _, _ string) (string, string, string, error) {
			t.Fatal("must not attempt to claim a secret for an internal agent")
			return "", "", "", nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.ClaimAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

// TestClaimAgentIdentitySecret_AgentNotFound_PropagatesError guards against a
// stale local binding surviving a best-effort cleanup: the claim must be
// rejected as soon as the agent's own source-of-truth record (OpenChoreo) is
// missing, without ever reaching the Thunder provisioning layer.
func TestClaimAgentIdentitySecret_AgentNotFound_PropagatesError(t *testing.T) {
	boom := errors.New("component not found")
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return nil, boom
		},
	}
	stub := &stubAgentThunderProvisioning{
		ClaimFunc: func(_ context.Context, _, _, _, _ string) (string, string, string, error) {
			t.Fatal("must not attempt to claim a secret when the agent itself cannot be fetched")
			return "", "", "", nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.ClaimAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.ErrorIs(t, err, boom)
}

// stubAgentConfigurationServiceForPromote implements AgentConfigurationService
// by embedding the (nil) interface and overriding only the two methods
// PromoteAgent actually calls — any other method call panics on the nil
// embed, which is fine since these tests never exercise them.
type stubAgentConfigurationServiceForPromote struct {
	AgentConfigurationService
	SystemKeysFunc func(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]bool, error)
	SystemVarsFunc func(ctx context.Context, agentID, ouID, projectName, environmentName string) ([]client.EnvVar, error)
}

func (s *stubAgentConfigurationServiceForPromote) ListSystemManagedEnvVarKeys(ctx context.Context, agentID, ouID, projectName, environmentName string) (map[string]bool, error) {
	return s.SystemKeysFunc(ctx, agentID, ouID, projectName, environmentName)
}

func (s *stubAgentConfigurationServiceForPromote) BuildSystemManagedEnvVarsFromConfig(ctx context.Context, agentID, ouID, projectName, environmentName string) ([]client.EnvVar, error) {
	return s.SystemVarsFunc(ctx, agentID, ouID, projectName, environmentName)
}

// promoteAgentTestFixture builds the minimal set of mocks PromoteAgent needs
// for a non-API-type internal agent (skips the large isAPIAgent branch
// entirely), for a dev -> staging promotion pipeline.
func promoteAgentTestFixture(t *testing.T, tgtIdentityEnvVars []client.EnvVar, tgtIdentityErr error) (*agentManagerService, *bool) {
	t.Helper()
	promoteCalled := false

	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: orgName}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
				Type:         models.AgentType{Type: "agent-chat"}, // deliberately not agent-api
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{PromotionPaths: []models.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "staging"}}},
			}}, nil
		},
		IsDeploymentInProgressFunc: func(_ context.Context, _, _, _ string) (bool, error) { return false, nil },
		PromoteComponentFunc: func(_ context.Context, _, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar, _ map[string]interface{}) error {
			promoteCalled = true
			return nil
		},
	}

	agentConfigSvc := &stubAgentConfigurationServiceForPromote{
		SystemKeysFunc: func(_ context.Context, _, _, _, _ string) (map[string]bool, error) { return map[string]bool{}, nil },
		SystemVarsFunc: func(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) { return nil, nil },
	}

	identityInjector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(_ context.Context, _, _, _, envName string) ([]client.EnvVar, error) {
			if tgtIdentityErr != nil {
				return nil, tgtIdentityErr
			}
			if envName == "staging" {
				return tgtIdentityEnvVars, nil
			}
			// Source environment's own identity vars must never be requested
			// by name for inclusion in the target — if PromoteAgent ever asks
			// this stub for "dev", something is wrong with the isolation logic.
			t.Fatalf("agentIdentityInjection.EnvVarsForEnvironment must only be called for the TARGET environment, got %q", envName)
			return nil, nil
		},
	}

	provisioningStub := &stubAgentThunderProvisioning{}
	// ProvisionForEnvironmentIfMissing is called unconditionally before the
	// readiness check — must not panic.
	provisioningStubWithProvision := &provisionForEnvIfMissingStub{stubAgentThunderProvisioning: provisioningStub}

	s := &agentManagerService{
		ocClient:                  ocClient,
		agentConfigurationService: agentConfigSvc,
		agentIdentityInjection:    identityInjector,
		agentThunderProvisioning:  provisioningStubWithProvision,
		logger:                    discardLogger(),
	}
	return s, &promoteCalled
}

// provisionForEnvIfMissingStub adds a no-op ProvisionForEnvironmentIfMissing
// on top of stubAgentThunderProvisioning, since PromoteAgent now calls it
// unconditionally before checking target-identity readiness.
type provisionForEnvIfMissingStub struct {
	*stubAgentThunderProvisioning
}

func (s *provisionForEnvIfMissingStub) ProvisionForEnvironmentIfMissing(_ context.Context, _, _, _, _ string, _ models.AgentProvisioningType, _ string) (bool, error) {
	return false, nil
}

func TestPromoteAgent_BlocksWhenTargetIdentityNotReady(t *testing.T) {
	// Empty, no-error result — exactly what EnvVarsForEnvironment returns
	// when the target's AgentID binding hasn't finished provisioning yet.
	s, promoteCalled := promoteAgentTestFixture(t, nil, nil)

	err := s.PromoteAgent(context.Background(), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "not ready yet")
	assert.False(t, *promoteCalled,
		"promotion must be blocked BEFORE calling PromoteComponent — otherwise the pod is already promoted with leaked credentials by the time this error is returned")
}

func TestPromoteAgent_TargetIdentityReady_PromotesWithTargetOnlyCredentials(t *testing.T) {
	targetVars := []client.EnvVar{
		{Key: "AMP_AGENT_IDENTITY_CLIENT_ID", Value: "staging-client-id"},
	}
	s, _ := promoteAgentTestFixture(t, targetVars, nil)

	var capturedOverrides []client.EnvVar
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.PromoteComponentFunc = func(_ context.Context, _, _, _, _, _ string, envOverrides []client.EnvVar, _ []client.FileVar, _ map[string]interface{}) error {
		capturedOverrides = envOverrides
		return nil
	}

	err := s.PromoteAgent(context.Background(), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.NoError(t, err)
	require.NotNil(t, capturedOverrides, "PromoteComponent must actually be called")

	found := false
	for _, ev := range capturedOverrides {
		if ev.Key == "AMP_AGENT_IDENTITY_CLIENT_ID" {
			found = true
			assert.Equal(t, "staging-client-id", ev.Value,
				"the target environment's own identity vars must be the ones actually sent to PromoteComponent")
		}
	}
	assert.True(t, found, "target environment's identity env vars must be present in the promoted overrides")
}

func TestPromoteAgent_IdentityBuildError_AbortsBeforePromoting(t *testing.T) {
	s, promoteCalled := promoteAgentTestFixture(t, nil, errors.New("openchoreo unavailable"))

	err := s.PromoteAgent(context.Background(), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	require.Error(t, err)
	assert.False(t, *promoteCalled, "a real error building identity env vars must abort before promoting, not just log a warning")
}
