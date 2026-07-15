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

// Unit tests for the agent label logic in agentManagerService: merging labels
// from the local sidecar table into responses, filtering ListAgents by label,
// and the update semantics of UpdateAgentBasicInfo. Follows the pattern of
// agent_kind_service_unit_test.go (moq mocks, no build tag, no database).
package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

const (
	testOrg     = "acme"
	testProject = "proj"
)

// newLabelTestService builds an agentManagerService with only the dependencies
// the label paths touch. Unconfigured mock methods panic, so a test reaching an
// unexpected collaborator fails loudly.
func newLabelTestService(oc *clientmocks.OpenChoreoClientMock, labelRepo *repomocks.AgentLabelRepositoryMock) *agentManagerService {
	return &agentManagerService{
		ocClient:       oc,
		agentLabelRepo: labelRepo,
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func ocMockWithComponents(agents []*models.AgentResponse) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListComponentsFunc: func(_ context.Context, _, _ string) ([]*models.AgentResponse, error) {
			return agents, nil
		},
	}
}

func TestAgentManagerService_ListAgents_Labels(t *testing.T) {
	agents := func() []*models.AgentResponse {
		return []*models.AgentResponse{
			{Name: "alpha"},
			{Name: "beta"},
			{Name: "gamma"},
		}
	}
	labelRows := []models.AgentLabel{
		{AgentName: "alpha", Labels: map[string]string{"env": "prod", "team": "ml"}},
		{AgentName: "beta", Labels: map[string]string{"env": "dev"}},
	}

	t.Run("merges labels into the listed agents", func(t *testing.T) {
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			ListByProjectFunc: func(_ context.Context, _, _ string) ([]models.AgentLabel, error) {
				return labelRows, nil
			},
		}
		svc := newLabelTestService(ocMockWithComponents(agents()), labelRepo)

		result, total, err := svc.ListAgents(context.Background(), testOrg, testProject, nil, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(3), total)
		require.Len(t, result, 3)
		assert.Equal(t, map[string]string{"env": "prod", "team": "ml"}, result[0].Labels)
		assert.Equal(t, map[string]string{"env": "dev"}, result[1].Labels)
		assert.Nil(t, result[2].Labels)
	})

	t.Run("filters by label before computing total and paginating", func(t *testing.T) {
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			ListByProjectFunc: func(_ context.Context, _, _ string) ([]models.AgentLabel, error) {
				return labelRows, nil
			},
		}
		svc := newLabelTestService(ocMockWithComponents(agents()), labelRepo)

		result, total, err := svc.ListAgents(context.Background(), testOrg, testProject, map[string]string{"env": "prod"}, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(1), total)
		require.Len(t, result, 1)
		assert.Equal(t, "alpha", result[0].Name)
	})

	t.Run("AND semantics: all filter labels must match", func(t *testing.T) {
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			ListByProjectFunc: func(_ context.Context, _, _ string) ([]models.AgentLabel, error) {
				return labelRows, nil
			},
		}
		svc := newLabelTestService(ocMockWithComponents(agents()), labelRepo)

		_, total, err := svc.ListAgents(context.Background(), testOrg, testProject, map[string]string{"env": "prod", "team": "web"}, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(0), total)
	})

	t.Run("label load failure fails a filtered list", func(t *testing.T) {
		boom := errors.New("db down")
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			ListByProjectFunc: func(_ context.Context, _, _ string) ([]models.AgentLabel, error) {
				return nil, boom
			},
		}
		svc := newLabelTestService(ocMockWithComponents(agents()), labelRepo)

		_, _, err := svc.ListAgents(context.Background(), testOrg, testProject, map[string]string{"env": "prod"}, 10, 0)

		assert.ErrorIs(t, err, boom)
	})

	t.Run("label load failure is tolerated for an unfiltered list", func(t *testing.T) {
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			ListByProjectFunc: func(_ context.Context, _, _ string) ([]models.AgentLabel, error) {
				return nil, errors.New("db down")
			},
		}
		svc := newLabelTestService(ocMockWithComponents(agents()), labelRepo)

		result, total, err := svc.ListAgents(context.Background(), testOrg, testProject, nil, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(3), total)
		require.Len(t, result, 3)
		assert.Nil(t, result[0].Labels)
	})
}

func TestAgentManagerService_GetAgent_Labels(t *testing.T) {
	ocMock := func() *clientmocks.OpenChoreoClientMock {
		return &clientmocks.OpenChoreoClientMock{
			GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
				return &models.OrganizationResponse{}, nil
			},
			GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
				return &models.AgentResponse{Name: "alpha"}, nil
			},
			// Erroring here skips the per-environment config block, which is
			// not under test.
			GetProjectDeploymentPipelineFunc: func(_ context.Context, _, _ string) (*models.DeploymentPipelineResponse, error) {
				return nil, errors.New("no pipeline in this test")
			},
		}
	}

	t.Run("attaches stored labels", func(t *testing.T) {
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			GetFunc: func(_ context.Context, _, _, _ string) (*models.AgentLabel, error) {
				return &models.AgentLabel{AgentName: "alpha", Labels: map[string]string{"env": "prod"}}, nil
			},
		}
		svc := newLabelTestService(ocMock(), labelRepo)

		agent, err := svc.GetAgent(context.Background(), testOrg, testProject, "alpha")

		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod"}, agent.Labels)
	})

	t.Run("tolerates a missing labels row", func(t *testing.T) {
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			GetFunc: func(_ context.Context, _, _, _ string) (*models.AgentLabel, error) {
				return nil, repositories.ErrAgentLabelsNotFound
			},
		}
		svc := newLabelTestService(ocMock(), labelRepo)

		agent, err := svc.GetAgent(context.Background(), testOrg, testProject, "alpha")

		require.NoError(t, err)
		assert.Nil(t, agent.Labels)
	})
}

func TestAgentManagerService_UpdateAgentBasicInfo_Labels(t *testing.T) {
	ocMock := func() *clientmocks.OpenChoreoClientMock {
		return &clientmocks.OpenChoreoClientMock{
			GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
				return &models.OrganizationResponse{}, nil
			},
			GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{}, nil
			},
			GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
				return &models.AgentResponse{Name: "alpha"}, nil
			},
			UpdateComponentBasicInfoFunc: func(_ context.Context, _, _, _ string, _ client.UpdateComponentBasicInfoRequest) error {
				return nil
			},
		}
	}
	req := func(labels *map[string]string) *spec.UpdateAgentBasicInfoRequest {
		return &spec.UpdateAgentBasicInfoRequest{
			DisplayName: "Alpha",
			Description: "desc",
			Labels:      labels,
		}
	}

	t.Run("nil labels leaves the sidecar table untouched", func(t *testing.T) {
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			GetFunc: func(_ context.Context, _, _, _ string) (*models.AgentLabel, error) {
				return nil, repositories.ErrAgentLabelsNotFound
			},
		}
		svc := newLabelTestService(ocMock(), labelRepo)

		_, err := svc.UpdateAgentBasicInfo(context.Background(), testOrg, testProject, "alpha", req(nil))

		require.NoError(t, err)
		// An Upsert call would panic (unconfigured mock); reaching here proves
		// it was never invoked.
		assert.Empty(t, labelRepo.UpsertCalls())
	})

	t.Run("provided labels are upserted and returned", func(t *testing.T) {
		var persisted *models.AgentLabel
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			UpsertFunc: func(_ context.Context, record *models.AgentLabel) error {
				persisted = record
				return nil
			},
		}
		svc := newLabelTestService(ocMock(), labelRepo)

		labels := map[string]string{"env": "prod"}
		agent, err := svc.UpdateAgentBasicInfo(context.Background(), testOrg, testProject, "alpha", req(&labels))

		require.NoError(t, err)
		require.NotNil(t, persisted)
		assert.Equal(t, testOrg, persisted.OUID)
		assert.Equal(t, testProject, persisted.ProjectName)
		assert.Equal(t, "alpha", persisted.AgentName)
		assert.Equal(t, labels, persisted.Labels)
		assert.Equal(t, labels, agent.Labels)
	})

	t.Run("upsert failure is surfaced", func(t *testing.T) {
		boom := errors.New("db down")
		labelRepo := &repomocks.AgentLabelRepositoryMock{
			UpsertFunc: func(_ context.Context, _ *models.AgentLabel) error {
				return boom
			},
		}
		svc := newLabelTestService(ocMock(), labelRepo)

		labels := map[string]string{"env": "prod"}
		_, err := svc.UpdateAgentBasicInfo(context.Background(), testOrg, testProject, "alpha", req(&labels))

		assert.ErrorIs(t, err, boom)
	})
}
