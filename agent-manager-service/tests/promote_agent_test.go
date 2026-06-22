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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testPromoteOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

// pipelineWithPath returns a mock GetProjectDeploymentPipeline func that allows source -> target.
func pipelineWithPath(source, target string) func(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error) {
	return func(ctx context.Context, namespaceName, projectName string) (*models.DeploymentPipelineResponse, error) {
		return &models.DeploymentPipelineResponse{
			Name:        "test-pipeline",
			DisplayName: "test-pipeline",
			OrgName:     namespaceName,
			CreatedAt:   time.Now(),
			PromotionPaths: []models.PromotionPath{
				{
					SourceEnvironmentRef:  source,
					TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: target}},
				},
			},
		}, nil
	}
}

// envVarValue returns the value of the env var with the given key, or "" if absent.
func envVarValue(envVars []client.EnvVar, key string) string {
	for _, ev := range envVars {
		if ev.Key == key {
			return ev.Value
		}
	}
	return ""
}

func TestPromoteAgent(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])

	promoteURL := func(org string) string {
		return fmt.Sprintf("/api/v1/orgs/%s/projects/my-project/agents/%s/promote", org, agentName)
	}

	t.Run("promoting along a valid path returns 202", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "production")
		// System-managed env var resolution parses the environment UUID, so it must be valid.
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		ocClient.PromoteComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, traitEnvConfigs map[string]interface{}) error {
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
		require.Len(t, ocClient.PromoteComponentCalls(), 1)
		call := ocClient.PromoteComponentCalls()[0]
		require.Equal(t, "development", call.SourceEnvironment)
		require.Equal(t, "production", call.TargetEnvironment)
	})

	t.Run("with useConfigFromSourceEnv=true clones the source env overrides", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "production")
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		// The source env's workload overrides are what get cloned to the target.
		ocClient.GetSourceEnvWorkloadOverridesFunc = func(ctx context.Context, namespaceName, componentName, sourceEnvironment string) ([]client.EnvVar, []client.FileVar, error) {
			return []client.EnvVar{{Key: "FROM_SOURCE", Value: "src-value"}},
				[]client.FileVar{{Key: "config.yaml", MountPath: "/etc/config.yaml", Value: "k: v"}}, nil
		}
		var capturedEnv []client.EnvVar
		var capturedFiles []client.FileVar
		ocClient.PromoteComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, traitEnvConfigs map[string]interface{}) error {
			capturedEnv = envOverrides
			capturedFiles = fileOverrides
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{
			SourceEnvironment:      "development",
			TargetEnvironment:      "production",
			UseConfigFromSourceEnv: boolPtr(true),
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
		// Source overrides must have been read and forwarded to the promote call.
		require.Len(t, ocClient.GetSourceEnvWorkloadOverridesCalls(), 1)
		require.Len(t, ocClient.PromoteComponentCalls(), 1)
		require.Equal(t, "src-value", envVarValue(capturedEnv, "FROM_SOURCE"))
		require.Len(t, capturedFiles, 1)
		require.Equal(t, "config.yaml", capturedFiles[0].Key)
	})

	t.Run("with useConfigFromSourceEnv=false forwards the provided env overrides", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "production")
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		var capturedEnv []client.EnvVar
		ocClient.PromoteComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName, sourceEnvironment, targetEnvironment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, traitEnvConfigs map[string]interface{}) error {
			capturedEnv = envOverrides
			return nil
		}
		testClients := wiring.TestClients{
			OpenChoreoClient: ocClient,
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		payload := spec.PromoteAgentRequest{
			SourceEnvironment:      "development",
			TargetEnvironment:      "production",
			UseConfigFromSourceEnv: boolPtr(false),
			Env:                    []spec.EnvironmentVariable{{Key: "LOG_LEVEL", Value: stringPtr("debug")}},
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
		// The source env's overrides must NOT have been read in the user-driven branch.
		require.Empty(t, ocClient.GetSourceEnvWorkloadOverridesCalls())
		require.Len(t, ocClient.PromoteComponentCalls(), 1)
		require.Equal(t, "debug", envVarValue(capturedEnv, "LOG_LEVEL"))
	})

	t.Run("returns 400 when sourceEnvironment is missing", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "sourceEnvironment is required")
		require.Empty(t, ocClient.PromoteComponentCalls())
	})

	t.Run("returns 400 when source and target are the same", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "development"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "must be different")
	})

	t.Run("returns 400 when useConfigFromSourceEnv is combined with overrides", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		// PromoteComponent must never be invoked: the request is rejected at validation,
		// so calling it would panic the nil mock func and fail the test.
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{
			SourceEnvironment:      "development",
			TargetEnvironment:      "production",
			UseConfigFromSourceEnv: boolPtr(true),
			Env:                    []spec.EnvironmentVariable{{Key: "FOO", Value: stringPtr("bar")}},
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		var errResp spec.ErrorResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
		require.Equal(t, utils.ErrCodeValidation, errResp.Code)
		require.Equal(t,
			"useConfigFromSourceEnv=true is mutually exclusive with env, files, enableAutoInstrumentation, enableApiKeySecurity, corsConfig, enableOAuthSecurity, and oauthConfig",
			errResp.Message)

		// The agent must not have been promoted.
		require.Empty(t, ocClient.PromoteComponentCalls())
	})

	t.Run("returns 500 when the promotion path is not allowed", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		// Pipeline only allows development -> staging, so development -> production is invalid.
		ocClient.GetProjectDeploymentPipelineFunc = pipelineWithPath("development", "staging")
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL(testPromoteOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
		require.Empty(t, ocClient.PromoteComponentCalls())
	})

	t.Run("returns 404 when the organization is not found", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.PromoteAgentRequest{SourceEnvironment: "development", TargetEnvironment: "production"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, promoteURL("nonexistent-org"), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), "Organization not found")
	})
}
