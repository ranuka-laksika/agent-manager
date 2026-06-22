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

var testEnvOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

// boolPtr is a helper to create a *bool.
func boolPtr(b bool) *bool { return &b }

// modelGatewayFromClient converts an OpenChoreo-client gateway spec back into the internal model
// type, mirroring what a real OpenChoreo response would echo. Used to exercise the gateway
// round-trip through the create-environment response.
func modelGatewayFromClient(g *client.GatewaySpec) *models.GatewaySpec {
	if g == nil {
		return nil
	}
	convListener := func(l *client.GatewayListenerSpec) *models.GatewayListenerSpec {
		if l == nil {
			return nil
		}
		return &models.GatewayListenerSpec{Port: l.Port, Host: l.Host}
	}
	convEndpoint := func(e *client.GatewayEndpointSpec) *models.GatewayEndpointSpec {
		if e == nil {
			return nil
		}
		return &models.GatewayEndpointSpec{
			HTTP:  convListener(e.HTTP),
			HTTPS: convListener(e.HTTPS),
			TLS:   convListener(e.TLS),
		}
	}
	convNetwork := func(n *client.GatewayNetworkSpec) *models.GatewayNetworkSpec {
		if n == nil {
			return nil
		}
		return &models.GatewayNetworkSpec{
			External: convEndpoint(n.External),
			Internal: convEndpoint(n.Internal),
		}
	}
	return &models.GatewaySpec{
		Ingress: convNetwork(g.Ingress),
		Egress:  convNetwork(g.Egress),
	}
}

func TestCreateEnvironment(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("creating an environment with valid data returns 201", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.CreateEnvironmentFunc = func(ctx context.Context, namespaceName string, req client.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{
				UUID:         uuid.New().String(),
				Name:         req.Name,
				DisplayName:  req.DisplayName,
				DataplaneRef: req.DataplaneRef,
				IsProduction: req.IsProduction,
				CreatedAt:    time.Now(),
			}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.CreateEnvironmentRequest{
			Name:         "staging",
			DisplayName:  "Staging",
			Description:  stringPtr("Staging environment"),
			DataplaneRef: "default-dataplane",
			DnsPrefix:    "staging",
			IsProduction: boolPtr(false),
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments", testEnvOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusCreated, rr.Code)

		var resp spec.GatewayEnvironmentResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, payload.Name, resp.Name)
		require.Equal(t, payload.DisplayName, resp.DisplayName)
		require.Equal(t, testEnvOrgName, resp.OrganizationName)

		require.Len(t, ocClient.CreateEnvironmentCalls(), 1)
		call := ocClient.CreateEnvironmentCalls()[0]
		require.Equal(t, testEnvOrgName, call.NamespaceName)
		require.Equal(t, payload.Name, call.Req.Name)
		require.Equal(t, payload.DataplaneRef, call.Req.DataplaneRef)
	})

	t.Run("creating an environment with a gateway spec forwards it to OpenChoreo", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.CreateEnvironmentFunc = func(ctx context.Context, namespaceName string, req client.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{
				UUID:         uuid.New().String(),
				Name:         req.Name,
				DisplayName:  req.DisplayName,
				DataplaneRef: req.DataplaneRef,
				IsProduction: req.IsProduction,
				// Echo the gateway back so the response round-trip is exercised.
				Gateway:   modelGatewayFromClient(req.Gateway),
				CreatedAt: time.Now(),
			}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.CreateEnvironmentRequest{
			Name:         "edge",
			DisplayName:  "Edge",
			DataplaneRef: "default-dataplane",
			DnsPrefix:    "edge",
			Gateway: &spec.GatewaySpec{
				Ingress: &spec.GatewayNetworkSpec{
					External: &spec.GatewayEndpointSpec{
						Http: &spec.GatewayListenerSpec{Port: int32Ptr(8080), Host: stringPtr("edge.example.com")},
					},
				},
			},
		}
		body, err := json.Marshal(payload)
		require.NoError(t, err)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments", testEnvOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusCreated, rr.Code)

		// The gateway spec must reach the OpenChoreo client intact.
		require.Len(t, ocClient.CreateEnvironmentCalls(), 1)
		gw := ocClient.CreateEnvironmentCalls()[0].Req.Gateway
		require.NotNil(t, gw)
		require.NotNil(t, gw.Ingress)
		require.NotNil(t, gw.Ingress.External)
		require.NotNil(t, gw.Ingress.External.HTTP)
		require.NotNil(t, gw.Ingress.External.HTTP.Port)
		require.Equal(t, int32(8080), *gw.Ingress.External.HTTP.Port)
		require.NotNil(t, gw.Ingress.External.HTTP.Host)
		require.Equal(t, "edge.example.com", *gw.Ingress.External.HTTP.Host)

		// And it must round-trip back into the response.
		var resp spec.GatewayEnvironmentResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.NotNil(t, resp.Gateway)
		require.NotNil(t, resp.Gateway.Ingress.External.Http)
		require.Equal(t, int32(8080), *resp.Gateway.Ingress.External.Http.Port)
	})

	t.Run("returns 400 on invalid request body", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments", testEnvOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString("{invalid-json"))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Empty(t, ocClient.CreateEnvironmentCalls())
	})

	t.Run("returns 409 when environment already exists", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.CreateEnvironmentFunc = func(ctx context.Context, namespaceName string, req client.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
			return nil, utils.ErrEnvironmentAlreadyExists
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.CreateEnvironmentRequest{Name: "staging", DisplayName: "Staging", DataplaneRef: "default-dataplane", DnsPrefix: "staging"}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/environments", testEnvOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusConflict, rr.Code)
		require.Contains(t, rr.Body.String(), "Environment already exists")
	})
}

func TestGetEnvironment(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("returns 200 for an existing environment", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{
				UUID:        uuid.New().String(),
				Name:        environmentName,
				DisplayName: "Development",
			}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments/development", testEnvOrgName)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		var resp spec.GatewayEnvironmentResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, "development", resp.Name)
	})

	t.Run("returns 404 when environment is not found", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return nil, utils.ErrEnvironmentNotFound
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments/missing", testEnvOrgName)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), "Environment not found")
	})
}

func TestListEnvironments(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("returns 200 with the list of environments", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.ListEnvironmentsFunc = func(ctx context.Context, namespaceName string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{
				{UUID: uuid.New().String(), Name: "development", DisplayName: "Development"},
				{UUID: uuid.New().String(), Name: "production", DisplayName: "Production", IsProduction: true},
			}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments", testEnvOrgName)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		var resp []spec.GatewayEnvironmentResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Len(t, resp, 2)
	})

	t.Run("returns 400 on invalid limit parameter", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments?limit=999999", testEnvOrgName)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestUpdateEnvironment(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("returns 200 on a successful update", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.UpdateEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string, req client.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
			displayName := environmentName
			if req.DisplayName != nil {
				displayName = *req.DisplayName
			}
			return &models.EnvironmentResponse{
				UUID:         uuid.New().String(),
				Name:         environmentName,
				DisplayName:  displayName,
				IsProduction: req.IsProduction != nil && *req.IsProduction,
			}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateEnvironmentRequest{DisplayName: stringPtr("Renamed Dev"), IsProduction: boolPtr(true)}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/environments/development", testEnvOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		var resp spec.GatewayEnvironmentResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Equal(t, "Renamed Dev", resp.DisplayName)
		require.True(t, resp.IsProduction)
		require.Len(t, ocClient.UpdateEnvironmentCalls(), 1)
	})

	t.Run("returns 404 when updating a missing environment", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.UpdateEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string, req client.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
			return nil, utils.ErrEnvironmentNotFound
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateEnvironmentRequest{DisplayName: stringPtr("X")}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/environments/missing", testEnvOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
	})
}

func TestDeleteEnvironment(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("returns 204 on a successful delete", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		ocClient.DeleteEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) error {
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments/development", testEnvOrgName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNoContent, rr.Code)
		require.Len(t, ocClient.DeleteEnvironmentCalls(), 1)
	})

	t.Run("returns 409 when a deployment pipeline references the environment", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		ocClient.ListDeploymentPipelinesFunc = func(ctx context.Context, namespaceName string) ([]*models.DeploymentPipelineResponse, error) {
			return []*models.DeploymentPipelineResponse{
				{
					Name:    "referencing-pipeline",
					OrgName: namespaceName,
					PromotionPaths: []models.PromotionPath{
						{
							SourceEnvironmentRef:  "development",
							TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "production"}},
						},
					},
				},
			}, nil
		}
		deleteCalled := false
		ocClient.DeleteEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) error {
			deleteCalled = true
			return nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments/development", testEnvOrgName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusConflict, rr.Code)
		require.False(t, deleteCalled, "OpenChoreo delete should not be called when the environment is still referenced")
		require.Empty(t, ocClient.DeleteEnvironmentCalls())
	})

	t.Run("returns 404 when deleting a missing environment", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return nil, utils.ErrEnvironmentNotFound
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/environments/missing", testEnvOrgName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Empty(t, ocClient.DeleteEnvironmentCalls())
	})
}
