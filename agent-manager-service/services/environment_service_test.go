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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestPipelineReferencesEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *models.DeploymentPipelineResponse
		envName  string
		want     bool
	}{
		{
			name:     "no promotion paths",
			pipeline: &models.DeploymentPipelineResponse{Name: "p"},
			envName:  "development",
			want:     false,
		},
		{
			name: "matches source environment",
			pipeline: &models.DeploymentPipelineResponse{
				PromotionPaths: []models.PromotionPath{{SourceEnvironmentRef: "development"}},
			},
			envName: "development",
			want:    true,
		},
		{
			name: "matches target environment",
			pipeline: &models.DeploymentPipelineResponse{
				PromotionPaths: []models.PromotionPath{{
					SourceEnvironmentRef:  "development",
					TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "production"}},
				}},
			},
			envName: "production",
			want:    true,
		},
		{
			name: "no match",
			pipeline: &models.DeploymentPipelineResponse{
				PromotionPaths: []models.PromotionPath{{
					SourceEnvironmentRef:  "development",
					TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "production"}},
				}},
			},
			envName: "staging",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, pipelineReferencesEnvironment(tt.pipeline, tt.envName))
		})
	}
}

func TestDeleteEnvironment_BlockedByDeploymentPipeline(t *testing.T) {
	const (
		orgName = "test-org"
		envName = "development"
	)

	t.Run("blocks deletion when a pipeline references the environment", func(t *testing.T) {
		deleteCalled := false
		ocClient := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, environmentName string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, namespaceName string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{
					{
						Name:    "referencing-pipeline",
						OrgName: namespaceName,
						PromotionPaths: []models.PromotionPath{
							{SourceEnvironmentRef: envName},
						},
					},
				}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				deleteCalled = true
				return nil
			},
		}
		// gatewayRepo is nil because the reference check returns before any local cleanup.
		svc := NewEnvironmentService(slog.Default(), nil, ocClient)

		err := svc.DeleteEnvironment(context.Background(), orgName, envName)

		require.ErrorIs(t, err, utils.ErrEnvironmentInUse)
		require.Contains(t, err.Error(), "referencing-pipeline")
		require.False(t, deleteCalled, "OpenChoreo delete must not be called when the environment is referenced")
		require.Empty(t, ocClient.DeleteEnvironmentCalls())
	})

	t.Run("propagates the error when listing pipelines fails", func(t *testing.T) {
		listErr := errors.New("openchoreo unavailable")
		deleteCalled := false
		ocClient := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, environmentName string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return nil, listErr
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				deleteCalled = true
				return nil
			},
		}
		svc := NewEnvironmentService(slog.Default(), nil, ocClient)

		err := svc.DeleteEnvironment(context.Background(), orgName, envName)

		require.ErrorIs(t, err, listErr)
		require.False(t, deleteCalled, "OpenChoreo delete must not be called when the reference check fails")
	})
}
