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

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestDeleteOrgDeploymentPipeline(t *testing.T) {
	const (
		orgName      = "test-org"
		pipelineName = "my-pipeline"
	)

	t.Run("blocks deletion when a project references the pipeline", func(t *testing.T) {
		deleteCalled := false
		ocClient := &clientmocks.OpenChoreoClientMock{
			ListProjectsFunc: func(_ context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
				return []*models.ProjectResponse{
					{Name: "other-project", OrgName: namespaceName, DeploymentPipeline: "another-pipeline"},
					{Name: "referencing-project", OrgName: namespaceName, DeploymentPipeline: pipelineName},
				}, nil
			},
			DeleteOrgDeploymentPipelineFunc: func(_ context.Context, _, _ string) error {
				deleteCalled = true
				return nil
			},
		}
		mgr := NewInfraResourceManager(ocClient, slog.Default())

		err := mgr.DeleteOrgDeploymentPipeline(context.Background(), orgName, pipelineName)

		require.ErrorIs(t, err, utils.ErrDeploymentPipelineInUse)
		require.Contains(t, err.Error(), "referencing-project")
		require.False(t, deleteCalled, "OpenChoreo delete must not be called when the pipeline is referenced")
		require.Empty(t, ocClient.DeleteOrgDeploymentPipelineCalls())
	})

	t.Run("deletes when no project references the pipeline", func(t *testing.T) {
		ocClient := &clientmocks.OpenChoreoClientMock{
			ListProjectsFunc: func(_ context.Context, namespaceName string) ([]*models.ProjectResponse, error) {
				return []*models.ProjectResponse{
					{Name: "other-project", OrgName: namespaceName, DeploymentPipeline: "another-pipeline"},
				}, nil
			},
			DeleteOrgDeploymentPipelineFunc: func(_ context.Context, _, _ string) error {
				return nil
			},
		}
		mgr := NewInfraResourceManager(ocClient, slog.Default())

		err := mgr.DeleteOrgDeploymentPipeline(context.Background(), orgName, pipelineName)

		require.NoError(t, err)
		require.Len(t, ocClient.DeleteOrgDeploymentPipelineCalls(), 1)
		require.Equal(t, pipelineName, ocClient.DeleteOrgDeploymentPipelineCalls()[0].PipelineName)
	})

	t.Run("propagates the error when listing projects fails", func(t *testing.T) {
		listErr := errors.New("openchoreo unavailable")
		deleteCalled := false
		ocClient := &clientmocks.OpenChoreoClientMock{
			ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
				return nil, listErr
			},
			DeleteOrgDeploymentPipelineFunc: func(_ context.Context, _, _ string) error {
				deleteCalled = true
				return nil
			},
		}
		mgr := NewInfraResourceManager(ocClient, slog.Default())

		err := mgr.DeleteOrgDeploymentPipeline(context.Background(), orgName, pipelineName)

		require.ErrorIs(t, err, listErr)
		require.False(t, deleteCalled, "OpenChoreo delete must not be called when the reference check fails")
	})
}
