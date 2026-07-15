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
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
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
	RegenerateFunc     func(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error)
	ClaimFunc          func(ctx context.Context, orgName, projectName, agentName, envName string) (string, string, string, error)
	GetAgentRolesFunc  func(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error)
	GetAgentGroupsFunc func(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error)
}

func (s *stubAgentThunderProvisioning) RegenerateSecret(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error) {
	return s.RegenerateFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) ClaimSecret(ctx context.Context, orgName, projectName, agentName, envName string) (string, string, string, error) {
	return s.ClaimFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetAgentRoles(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error) {
	return s.GetAgentRolesFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetAgentGroups(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error) {
	return s.GetAgentGroupsFunc(ctx, orgName, projectName, agentName, envName)
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
	s := &agentManagerService{agentThunderProvisioning: stub}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeInternal, resp.ProvisioningType)
	assert.Equal(t, "fresh-secret-internal", resp.ClientSecret,
		"an Internal agent must ALSO get its freshly regenerated secret back — regenerate is not the "+
			"one-time-claim endpoint, withholding it here would just force a second call to see it")
	assert.Equal(t, models.AgentRegenerateSecretStatus, resp.Status)
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

func pipelineWithEnv(envName string) *models.DeploymentPipelineResponse {
	return &models.DeploymentPipelineResponse{
		PromotionPaths: []models.PromotionPath{
			{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: envName}}},
		},
	}
}

func TestGetAgentRoles_EnvironmentInPipeline_DelegatesToProvisioning(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	wantRoles := []thundersvc.ThunderRole{{ID: "role-1", Name: "reader"}}
	stub := &stubAgentThunderProvisioning{
		GetAgentRolesFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderRole, error) {
			return wantRoles, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	roles, err := s.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, wantRoles, roles)
}

// TestGetAgentRoles_EnvironmentNotInPipeline_Errors guards the same visibility
// rule GetAgentIdentity applies: a project only ever sees bindings for
// environments in its own deployment pipeline, even though AgentIDs are
// provisioned across every org-level environment.
func TestGetAgentRoles_EnvironmentNotInPipeline_Errors(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	stub := &stubAgentThunderProvisioning{
		GetAgentRolesFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderRole, error) {
			t.Fatal("must not reach the provisioning layer for an environment outside the project's pipeline")
			return nil, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "prod")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

func TestGetAgentGroups_EnvironmentInPipeline_DelegatesToProvisioning(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	wantGroups := []thundersvc.ThunderGroup{{ID: "group-1", Name: "operators"}}
	stub := &stubAgentThunderProvisioning{
		GetAgentGroupsFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderGroup, error) {
			return wantGroups, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	groups, err := s.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "staging")

	require.NoError(t, err)
	assert.Equal(t, wantGroups, groups)
}

// TestGetAgentGroups_EnvironmentNotInPipeline_Errors is the groups counterpart
// to TestGetAgentRoles_EnvironmentNotInPipeline_Errors.
func TestGetAgentGroups_EnvironmentNotInPipeline_Errors(t *testing.T) {
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return pipelineWithEnv("staging"), nil
		},
	}
	stub := &stubAgentThunderProvisioning{
		GetAgentGroupsFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderGroup, error) {
			t.Fatal("must not reach the provisioning layer for an environment outside the project's pipeline")
			return nil, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "prod")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

// Labels now live on the OpenChoreo component itself rather than a local
// sidecar table; these tests cover the service-layer wiring that threads
// labels into/out of the OpenChoreo client requests. The label-merge/extract
// logic itself is tested at the client-package level
// (clients/openchoreosvc/client/component_labels_test.go).

func TestToCreateAgentRequestWithSecrets_PassesLabelsThrough(t *testing.T) {
	s := &agentManagerService{}

	t.Run("labels set", func(t *testing.T) {
		labels := map[string]string{"team": "ml"}
		req := &spec.CreateAgentRequest{Name: "agent-1", DisplayName: "Agent 1", Labels: &labels}

		result := s.toCreateAgentRequestWithSecrets(req, "")

		assert.Equal(t, labels, result.Labels)
	})

	t.Run("labels nil", func(t *testing.T) {
		req := &spec.CreateAgentRequest{Name: "agent-1", DisplayName: "Agent 1"}

		result := s.toCreateAgentRequestWithSecrets(req, "")

		assert.Nil(t, result.Labels)
	})
}

func TestUpdateAgentBasicInfo_PassesLabelsThroughToClient(t *testing.T) {
	newLabels := map[string]string{"team": "ml"}
	emptyLabels := map[string]string{}

	testCases := []struct {
		name       string
		reqLabels  *map[string]string
		wantLabels *map[string]string
	}{
		{name: "nil means unchanged", reqLabels: nil, wantLabels: nil},
		{name: "empty map clears user labels", reqLabels: &emptyLabels, wantLabels: &emptyLabels},
		{name: "populated map replaces user labels", reqLabels: &newLabels, wantLabels: &newLabels},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var captured client.UpdateComponentBasicInfoRequest
			ocClient := &clientmocks.OpenChoreoClientMock{
				GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
					return &models.OrganizationResponse{}, nil
				},
				GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
					return &models.ProjectResponse{}, nil
				},
				GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
					return &models.AgentResponse{Name: "agent-1"}, nil
				},
				UpdateComponentBasicInfoFunc: func(_ context.Context, _, _, _ string, req client.UpdateComponentBasicInfoRequest) error {
					captured = req
					return nil
				},
			}
			s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

			_, err := s.UpdateAgentBasicInfo(context.Background(), "acme", "proj1", "agent-1", &spec.UpdateAgentBasicInfoRequest{
				DisplayName: "Agent 1",
				Description: "desc",
				Labels:      tc.reqLabels,
			})

			require.NoError(t, err)
			assert.Equal(t, tc.wantLabels, captured.Labels)
		})
	}
}

func TestGetAgent_LabelsPassThroughUnmodified(t *testing.T) {
	wantLabels := map[string]string{"team": "ml"}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Name: "agent-1", Labels: wantLabels}, nil
		},
		GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
			return nil, errors.New("no pipeline in this test")
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	agent, err := s.GetAgent(context.Background(), "acme", "proj1", "agent-1")

	require.NoError(t, err)
	assert.Equal(t, wantLabels, agent.Labels)
}

func TestListAgents_LabelsPassThroughUnmodified(t *testing.T) {
	agents := []*models.AgentResponse{
		{Name: "agent-1", Labels: map[string]string{"env": "prod"}},
		{Name: "agent-2", Labels: map[string]string{"env": "dev"}},
	}
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListComponentsFunc: func(_ context.Context, _, _ string) ([]*models.AgentResponse, error) {
			return agents, nil
		},
	}
	s := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	t.Run("unfiltered returns all agents with their labels intact", func(t *testing.T) {
		result, total, err := s.ListAgents(context.Background(), "acme", "proj1", nil, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(2), total)
		require.Len(t, result, 2)
		assert.Equal(t, map[string]string{"env": "prod"}, result[0].Labels)
	})

	t.Run("filtered narrows to the matching agent", func(t *testing.T) {
		result, total, err := s.ListAgents(context.Background(), "acme", "proj1", map[string]string{"env": "prod"}, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(1), total)
		require.Len(t, result, 1)
		assert.Equal(t, "agent-1", result[0].Name)
	})
}
