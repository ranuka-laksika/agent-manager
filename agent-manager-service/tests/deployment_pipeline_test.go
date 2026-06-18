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

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testPipelineOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

func validPromotionPaths() []spec.PromotionPath {
	return []spec.PromotionPath{
		{
			SourceEnvironmentRef: "development",
			TargetEnvironmentRefs: []spec.TargetEnvironmentRef{
				{Name: "production"},
			},
		},
	}
}

func TestCreateDeploymentPipeline(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("creating a pipeline with valid data returns 201", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.CreateDeploymentPipelineFunc = func(ctx context.Context, namespaceName, pipelineName string, displayName, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
			dn := ""
			if displayName != nil {
				dn = *displayName
			}
			return &models.DeploymentPipelineResponse{
				Name:           pipelineName,
				DisplayName:    dn,
				OrgName:        namespaceName,
				CreatedAt:      time.Now(),
				PromotionPaths: promotionPaths,
			}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.CreateDeploymentPipelineRequest{
			DisplayName:    "My Pipeline",
			Description:    stringPtr("a pipeline"),
			PromotionPaths: validPromotionPaths(),
		}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusCreated, rr.Code)
		var resp spec.DeploymentPipelineResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, "My Pipeline", resp.DisplayName)
		require.Len(t, resp.PromotionPaths, 1)

		require.Len(t, ocClient.CreateDeploymentPipelineCalls(), 1)
		call := ocClient.CreateDeploymentPipelineCalls()[0]
		require.Equal(t, testPipelineOrgName, call.NamespaceName)
		// pipelineName is slugified from the display name
		require.Equal(t, "my-pipeline", call.PipelineName)
	})

	t.Run("returns 400 when displayName is missing", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.CreateDeploymentPipelineRequest{PromotionPaths: validPromotionPaths()}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "displayName is required")
		require.Empty(t, ocClient.CreateDeploymentPipelineCalls())
	})

	t.Run("returns 400 when promotionPaths is empty", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.CreateDeploymentPipelineRequest{DisplayName: "My Pipeline", PromotionPaths: []spec.PromotionPath{}}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "promotionPaths must contain at least one entry")
	})

	t.Run("returns 500 when OpenChoreo creation fails", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.CreateDeploymentPipelineFunc = func(ctx context.Context, namespaceName, pipelineName string, displayName, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
			return nil, fmt.Errorf("OpenChoreo service error")
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.CreateDeploymentPipelineRequest{DisplayName: "My Pipeline", PromotionPaths: validPromotionPaths()}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestUpdateDeploymentPipeline(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("returns 200 on a successful update", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.UpdateDeploymentPipelineFunc = func(ctx context.Context, namespaceName, pipelineName string, displayName, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
			dn := pipelineName
			if displayName != nil {
				dn = *displayName
			}
			return &models.DeploymentPipelineResponse{
				Name:           pipelineName,
				DisplayName:    dn,
				OrgName:        namespaceName,
				PromotionPaths: promotionPaths,
			}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateDeploymentPipelineRequest{DisplayName: stringPtr("Updated"), PromotionPaths: validPromotionPaths()}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/my-pipeline", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Len(t, ocClient.UpdateDeploymentPipelineCalls(), 1)
		require.Equal(t, "my-pipeline", ocClient.UpdateDeploymentPipelineCalls()[0].PipelineName)
	})

	t.Run("returns 400 when promotionPaths is empty", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateDeploymentPipelineRequest{DisplayName: stringPtr("Updated"), PromotionPaths: []spec.PromotionPath{}}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/my-pipeline", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Empty(t, ocClient.UpdateDeploymentPipelineCalls())
	})

	// A missing pipeline surfaces from the OpenChoreo client as a generic ErrNotFound, which
	// handleCommonErrors does not map specially, so the controller returns 500.
	t.Run("returns 500 when the pipeline is not found in OpenChoreo", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.UpdateDeploymentPipelineFunc = func(ctx context.Context, namespaceName, pipelineName string, displayName, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
			return nil, utils.ErrNotFound
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateDeploymentPipelineRequest{DisplayName: stringPtr("Updated"), PromotionPaths: validPromotionPaths()}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/missing", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestDeleteDeploymentPipeline(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("returns 204 on a successful delete", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.DeleteOrgDeploymentPipelineFunc = func(ctx context.Context, namespaceName, pipelineName string) error {
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/my-pipeline", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNoContent, rr.Code)
		require.Len(t, ocClient.DeleteOrgDeploymentPipelineCalls(), 1)
		require.Equal(t, "my-pipeline", ocClient.DeleteOrgDeploymentPipelineCalls()[0].PipelineName)
	})

	t.Run("returns 409 when a project still references the pipeline", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.ListProjectsFunc = func(ctx context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
			return []*models.ProjectResponse{
				{Name: "referencing-project", OrgName: namespaceName, DeploymentPipeline: "my-pipeline"},
			}, nil
		}
		deleteCalled := false
		ocClient.DeleteOrgDeploymentPipelineFunc = func(ctx context.Context, namespaceName, pipelineName string) error {
			deleteCalled = true
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/my-pipeline", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusConflict, rr.Code)
		require.False(t, deleteCalled, "OpenChoreo delete should not be called when the pipeline is still referenced")
		require.Empty(t, ocClient.DeleteOrgDeploymentPipelineCalls())
	})

	t.Run("returns 500 when the delete fails in OpenChoreo", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.DeleteOrgDeploymentPipelineFunc = func(ctx context.Context, namespaceName, pipelineName string) error {
			return fmt.Errorf("OpenChoreo service error")
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/missing", testPipelineOrgName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}
