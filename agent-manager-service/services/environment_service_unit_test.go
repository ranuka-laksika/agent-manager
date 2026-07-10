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

// UNIT tests for environmentService. Like agent_kind_service_unit_test.go (the
// reference), this file has NO `//go:build integration` tag, so it runs in the
// fast unit tier with the dependencies mocked:
//
//   - repositories.GatewayRepository -> repomocks.GatewayRepositoryMock
//   - occlient.OpenChoreoClient      -> clientmocks.OpenChoreoClientMock
//   - thundersvc.Prober              -> clientmocks.ThunderProberMock
//
// The goal is to exercise the service's OWN logic (error mapping to sentinels,
// validation gates, pagination, fan-out/aggregation, transformation) without a
// database. Unconfigured mock methods panic, so any unexpected call fails loudly.
package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// newEnvService wires the service with a discard logger and the two mocked deps.
// The Thunder prober is left unconfigured (panics if called) since only the
// ListThunderInstances tests exercise it — see newEnvServiceWithProber.
func newEnvService(repo *repomocks.GatewayRepositoryMock, oc *clientmocks.OpenChoreoClientMock) EnvironmentService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEnvironmentService(logger, repo, oc, &clientmocks.ThunderProberMock{}, nil)
}

// newEnvServiceWithProber is like newEnvService but with a configured Thunder prober,
// for tests that exercise ListThunderInstances' reachability branch.
func newEnvServiceWithProber(repo *repomocks.GatewayRepositoryMock, oc *clientmocks.OpenChoreoClientMock, prober thundersvc.Prober) EnvironmentService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEnvironmentService(logger, repo, oc, prober, nil)
}

// -----------------------------------------------------------------------------
// CreateEnvironment — wraps client errors; maps client response on success.
// -----------------------------------------------------------------------------

func TestEnvironmentService_CreateEnvironment(t *testing.T) {
	const org = "acme"

	t.Run("wraps a client error", func(t *testing.T) {
		boom := errors.New("oc unreachable")
		oc := &clientmocks.OpenChoreoClientMock{
			CreateEnvironmentFunc: func(_ context.Context, _ string, _ occlient.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.CreateEnvironment(context.Background(), org, &models.CreateEnvironmentRequest{Name: "dev"})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("maps the client response on success", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			CreateEnvironmentFunc: func(_ context.Context, ns string, req occlient.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				// The request must be translated from the public model.
				assert.Equal(t, org, ns)
				assert.Equal(t, "dev", req.Name)
				return &models.EnvironmentResponse{
					UUID:         "11111111-1111-1111-1111-111111111111",
					Name:         "dev",
					DisplayName:  "Development",
					DataplaneRef: "dp-1",
					IsProduction: true,
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.CreateEnvironment(context.Background(), org, &models.CreateEnvironmentRequest{
			Name:         "dev",
			DisplayName:  "Development",
			Description:  "desc from request",
			DataplaneRef: "dp-1",
			IsProduction: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "dev", resp.Name)
		assert.Equal(t, org, resp.OrganizationName)
		assert.Equal(t, "Development", resp.DisplayName)
		// Description on the response comes from the REQUEST, not the OC response.
		assert.Equal(t, "desc from request", resp.Description)
		assert.True(t, resp.IsProduction)
	})
}

// -----------------------------------------------------------------------------
// GetEnvironment — maps not-found to the sentinel; wraps everything else.
// -----------------------------------------------------------------------------

func TestEnvironmentService_GetEnvironment(t *testing.T) {
	const org, envID = "acme", "dev"

	t.Run("maps not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps an unexpected client error", func(t *testing.T) {
		boom := errors.New("connection reset")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("returns mapped response on success", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{
					UUID:        "uuid-1",
					Name:        "dev",
					DisplayName: "Development",
					Description: "from oc",
					DNSPrefix:   "dev-prefix",
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.GetEnvironment(context.Background(), org, envID)

		require.NoError(t, err)
		assert.Equal(t, "dev", resp.Name)
		assert.Equal(t, org, resp.OrganizationName)
		// Unlike Create, Get carries description + DNS prefix straight from OC.
		assert.Equal(t, "from oc", resp.Description)
		assert.Equal(t, "dev-prefix", resp.DNSPrefix)
	})
}

// -----------------------------------------------------------------------------
// ListEnvironments — exercises pagination (offset/limit) and aggregation.
// -----------------------------------------------------------------------------

func TestEnvironmentService_ListEnvironments(t *testing.T) {
	const org = "acme"

	threeEnvs := func() []*models.EnvironmentResponse {
		return []*models.EnvironmentResponse{
			{UUID: "u1", Name: "env-0"},
			{UUID: "u2", Name: "env-1"},
			{UUID: "u3", Name: "env-2"},
		}
	}

	t.Run("wraps a client error", func(t *testing.T) {
		boom := errors.New("oc down")
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.ListEnvironments(context.Background(), org, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("applies limit within the available range", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 2, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total) // total reflects all, not the page
		require.Len(t, resp.Environments, 2)
		assert.Equal(t, "env-0", resp.Environments[0].Name)
		assert.Equal(t, "env-1", resp.Environments[1].Name)
		assert.Equal(t, org, resp.Environments[0].OrganizationName)
	})

	t.Run("clamps the page end when limit overruns the slice", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 10, 1)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total)
		require.Len(t, resp.Environments, 2) // env-1, env-2
		assert.Equal(t, "env-1", resp.Environments[0].Name)
	})

	t.Run("returns an empty page when offset is past the end", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 10, 5)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total) // total still reports everything
		assert.Empty(t, resp.Environments)
		assert.Equal(t, int32(5), resp.Offset)
	})
}

// -----------------------------------------------------------------------------
// UpdateEnvironment — both ErrNotFound and ErrEnvironmentNotFound collapse to
// the env sentinel; other errors are wrapped. Description comes from the request.
// -----------------------------------------------------------------------------

func TestEnvironmentService_UpdateEnvironment(t *testing.T) {
	const org, envID = "acme", "dev"

	t.Run("maps generic not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{})

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps an unexpected client error", func(t *testing.T) {
		boom := errors.New("boom")
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("uses the request description on success", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{
					UUID:        "u1",
					Name:        "dev",
					Description: "ignored-oc-desc",
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{
			Description: strPtr("new description"),
		})

		require.NoError(t, err)
		assert.Equal(t, "new description", resp.Description)
		assert.Equal(t, org, resp.OrganizationName)
	})

	t.Run("defaults description to empty when request omits it", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: "u1", Name: "dev", Description: "oc-desc"}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{})

		require.NoError(t, err)
		assert.Equal(t, "", resp.Description)
	})
}

// -----------------------------------------------------------------------------
// pipelineReferencesEnvironment — the pure predicate DeleteEnvironment uses to
// decide whether a pipeline blocks deletion (as source or as a promotion target).
// -----------------------------------------------------------------------------

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
			assert.Equal(t, tt.want, pipelineReferencesEnvironment(tt.pipeline, tt.envName))
		})
	}
}

// -----------------------------------------------------------------------------
// DeleteEnvironment — the richest method: lookup, UUID parse, pipeline-reference
// guard, OC delete (idempotent on not-found), then local mapping cleanup.
// -----------------------------------------------------------------------------

func TestEnvironmentService_DeleteEnvironment(t *testing.T) {
	const org, envID = "acme", "dev"
	const envUUID = "22222222-2222-2222-2222-222222222222"

	t.Run("maps lookup not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("fails on an invalid UUID from OpenChoreo", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: "not-a-uuid", Name: "dev"}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("blocks deletion when a pipeline references the environment", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{
					{
						Name: "pipeline-a",
						PromotionPaths: []models.PromotionPath{
							{SourceEnvironmentRef: "dev"}, // references our env as source
						},
					},
				}, nil
			},
			// DeleteEnvironment must NOT be reached — leaving it nil asserts that.
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentInUse)
		assert.Contains(t, err.Error(), "pipeline-a")
		assert.Empty(t, oc.DeleteEnvironmentCalls())
	})

	t.Run("blocks deletion when referenced as a pipeline target", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{
					{
						Name: "pipeline-b",
						PromotionPaths: []models.PromotionPath{
							{
								SourceEnvironmentRef:  "staging",
								TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "dev"}},
							},
						},
					},
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentInUse)
	})

	t.Run("wraps a pipeline-listing error", func(t *testing.T) {
		boom := errors.New("pipelines unreachable")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return nil, boom
			},
			// DeleteEnvironment must NOT be reached — leaving it nil asserts that.
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.Empty(t, oc.DeleteEnvironmentCalls())
	})

	t.Run("surfaces a non-not-found OC delete error without local cleanup", func(t *testing.T) {
		boom := errors.New("release bindings still exist")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				return boom
			},
		}
		// DeleteEnvironmentMappingsByEnvironmentID must NOT be reached (nil asserts that).
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("treats an OC not-found delete as idempotent and still cleans up locally", func(t *testing.T) {
		cleaned := false
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				return utils.ErrEnvironmentNotFound
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			DeleteEnvironmentMappingsByEnvironmentIDFunc: func(_ string) (int64, error) {
				cleaned = true
				return 0, nil
			},
		}
		svc := newEnvService(repo, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.NoError(t, err)
		assert.True(t, cleaned, "expected local mapping cleanup to run")
	})

	t.Run("returns an error when local mapping cleanup fails", func(t *testing.T) {
		boom := errors.New("db down")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				return nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			DeleteEnvironmentMappingsByEnvironmentIDFunc: func(_ string) (int64, error) {
				return 0, boom
			},
		}
		svc := newEnvService(repo, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("happy path: OC delete then local cleanup", func(t *testing.T) {
		var cleanedID string
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{
					{Name: "unrelated", PromotionPaths: []models.PromotionPath{{SourceEnvironmentRef: "prod"}}},
				}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, name string) error {
				assert.Equal(t, "dev", name)
				return nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			DeleteEnvironmentMappingsByEnvironmentIDFunc: func(id string) (int64, error) {
				cleanedID = id
				return 2, nil
			},
		}
		svc := newEnvService(repo, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.NoError(t, err)
		assert.Equal(t, envUUID, cleanedID)
	})
}

// -----------------------------------------------------------------------------
// GetEnvironmentGateways — verify env, resolve mappings, fan-out per gateway,
// skip missing/errored gateways, and map IsActive -> status string.
// -----------------------------------------------------------------------------

func TestEnvironmentService_GetEnvironmentGateways(t *testing.T) {
	const org, envID = "acme", "dev"
	const envUUID = "33333333-3333-3333-3333-333333333333"

	t.Run("maps env not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps a mapping-lookup error", func(t *testing.T) {
		boom := errors.New("mapping query failed")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return nil, boom
			},
		}
		svc := newEnvService(repo, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("aggregates gateways and skips missing/errored ones", func(t *testing.T) {
		activeGW := uuid.New()
		errorGW := uuid.New()
		missingGW := uuid.New()

		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return []models.GatewayEnvironmentMapping{
					{GatewayUUID: activeGW},
					{GatewayUUID: errorGW},
					{GatewayUUID: missingGW},
				}, nil
			},
			GetByUUIDFunc: func(id string) (*models.Gateway, error) {
				switch id {
				case activeGW.String():
					return &models.Gateway{
						UUID:                     activeGW,
						Name:                     "gw-active",
						DisplayName:              "Active GW",
						GatewayFunctionalityType: "egress",
						Vhost:                    "gw.example.com",
						IsCritical:               true,
						IsActive:                 true,
					}, nil
				case errorGW.String():
					return nil, errors.New("transient") // skipped, not fatal
				case missingGW.String():
					//nolint:nilnil // intentionally exercising the (nil, nil) "missing, skip" input the service must handle
					return nil, nil // skipped
				default:
					return nil, errors.New("unexpected gateway id")
				}
			},
		}
		svc := newEnvService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1) // only the active gateway survives
		assert.Equal(t, "gw-active", resp[0].Name)
		assert.Equal(t, activeGW.String(), resp[0].UUID)
		assert.Equal(t, org, resp[0].OrganizationName)
		assert.True(t, resp[0].IsCritical)
		assert.Equal(t, string(models.GatewayStatusActive), resp[0].Status)
	})

	t.Run("maps an inactive gateway to inactive status", func(t *testing.T) {
		gw := uuid.New()
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return []models.GatewayEnvironmentMapping{{GatewayUUID: gw}}, nil
			},
			GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gw, Name: "gw-idle", IsActive: false}, nil
			},
		}
		svc := newEnvService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1)
		assert.Equal(t, string(models.GatewayStatusInactive), resp[0].Status)
	})
}

// -----------------------------------------------------------------------------
// ListThunderInstances — gates returning Thunder instance info on whether
// the env-Thunder JWKS endpoint is actually reachable (live HTTP probe).
// -----------------------------------------------------------------------------

func TestEnvironmentService_ListThunderInstances(t *testing.T) {
	const org = "acme"

	t.Run("wraps list environments error", func(t *testing.T) {
		boom := errors.New("oc down")
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.ListThunderInstances(context.Background(), org)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("skips environments with unreachable Thunder", func(t *testing.T) {
		// The prober reports every env as unreachable, so the result list must be
		// empty — proving that gateway mappings alone are NOT sufficient to advertise
		// Thunder endpoints.
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					{UUID: "u1", Name: "dev", DisplayName: "Dev", IsProduction: false},
					{UUID: "u2", Name: "staging", DisplayName: "Staging", IsProduction: false},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		prober := &clientmocks.ThunderProberMock{
			ProbeFunc: func(_ context.Context, _, _ string) bool { return false },
		}
		svc := newEnvServiceWithProber(&repomocks.GatewayRepositoryMock{}, oc, prober)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.ThunderInstances)
	})

	t.Run("skips nil and empty-name environments", func(t *testing.T) {
		// newEnvService's prober is unconfigured (panics if called) — proving the
		// nil/empty-name guard skips these entries before ever probing them.
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					nil,
					{UUID: "u1", Name: ""},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.ThunderInstances)
	})

	t.Run("includes reachable Thunder instances with correctly constructed URLs", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					{UUID: "u1", Name: "dev", DisplayName: "Dev", IsProduction: false},
					{UUID: "u2", Name: "staging", DisplayName: "Staging", IsProduction: true},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		prober := &clientmocks.ThunderProberMock{
			ProbeFunc: func(_ context.Context, _, _ string) bool { return true },
		}
		svc := newEnvServiceWithProber(&repomocks.GatewayRepositoryMock{}, oc, prober)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.ThunderInstances, 2)

		dev := resp.ThunderInstances[0]
		assert.Equal(t, "dev", dev.EnvName)
		assert.Equal(t, "Dev", dev.DisplayName)
		assert.False(t, dev.IsProduction)
		assert.Equal(t, thundersvc.ThunderIssuerURL(org, "dev"), dev.IssuerURL)
		assert.Equal(t, thundersvc.ThunderExternalTokenURL(org, "dev"), dev.TokenURL)
		assert.Equal(t, thundersvc.ThunderExternalJWKSURL(org, "dev"), dev.JWKSURL)
		assert.Equal(t, thundersvc.ThunderNamespace(org, "dev"), dev.Namespace)

		staging := resp.ThunderInstances[1]
		assert.Equal(t, "staging", staging.EnvName)
		assert.True(t, staging.IsProduction)
	})
}
