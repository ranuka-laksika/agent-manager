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
	"fmt"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// testKeyTTL is the validity window for a console-issued test API key.
// The console refreshes the key at staleTime well before this elapses.
const testKeyTTL = 10 * time.Minute

// AgentAPIKeyServiceInterface defines the contract for agent API key operations
type AgentAPIKeyServiceInterface interface {
	CreateAPIKey(ctx context.Context, orgName, projectName, agentName, envID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	RevokeAPIKey(ctx context.Context, orgName, projectName, agentName, envID, keyName string) error
	RotateAPIKey(ctx context.Context, orgName, projectName, agentName, envID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	ListAPIKeys(ctx context.Context, orgName, projectName, agentName, envID string) ([]models.StoredAPIKey, error)
	IssueTestAPIKey(ctx context.Context, orgName, projectName, agentName, envID string) (*models.IssueTestAPIKeyResponse, error)
}

// AgentAPIKeyService handles API key management for agents
type AgentAPIKeyService struct {
	artifactRepo repositories.ArtifactRepository
	ocClient     client.OpenChoreoClient
	apiKeyRepo   repositories.APIKeyRepository
	gatewayRepo  repositories.GatewayRepository
	broadcaster  apiKeyBroadcaster
}

// NewAgentAPIKeyService creates a new agent API key service instance
func NewAgentAPIKeyService(
	artifactRepo repositories.ArtifactRepository,
	ocClient client.OpenChoreoClient,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
) *AgentAPIKeyService {
	return &AgentAPIKeyService{
		artifactRepo: artifactRepo,
		ocClient:     ocClient,
		apiKeyRepo:   apiKeyRepo,
		gatewayRepo:  gatewayRepo,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
			apiKeyRepo:     apiKeyRepo,
		},
	}
}

func (s *AgentAPIKeyService) resolveAgentAPIArtifact(ctx context.Context, orgName, projectName, agentName, envID string) (*models.Artifact, string, error) {
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, envID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get environment: %w", translateEnvironmentError(err))
	}

	artifact, err := s.artifactRepo.GetByHandle(agentEnvAPIArtifactHandle(projectName, agentName, environment.UUID), orgName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get agent API artifact: %w", err)
	}
	if artifact.Kind != models.KindAgent {
		return nil, "", utils.ErrArtifactNotFound
	}
	return artifact, environment.UUID, nil
}

// resolveEnvGateways returns the gateways associated with the given environment UUID.
func (s *AgentAPIKeyService) resolveEnvGateways(envUUID string) ([]*models.Gateway, error) {
	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(envUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway mappings: %w", err)
	}
	var gateways []*models.Gateway
	for _, m := range mappings {
		gw, err := s.gatewayRepo.GetByUUID(m.GatewayUUID.String())
		if err != nil {
			return nil, fmt.Errorf("failed to get gateway %s: %w", m.GatewayUUID, err)
		}
		gateways = append(gateways, gw)
	}
	if len(gateways) == 0 {
		return nil, utils.ErrGatewayNotFound
	}
	return gateways, nil
}

// CreateAPIKey generates an API key for an agent and broadcasts it to the environment's gateways.
func (s *AgentAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgName, projectName, agentName, envID string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if req != nil && req.Name == models.APIKeyTestKeyName {
		return nil, fmt.Errorf("%w: %q is reserved for console test keys", utils.ErrBadRequest, models.APIKeyTestKeyName)
	}
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, orgName, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return nil, err
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastCreateToGateways(gateways, orgName, artifactUUID, artifactUUID, req)
}

// RevokeAPIKey broadcasts an API key revocation event to the environment's gateways.
func (s *AgentAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	orgName, projectName, agentName, envID, keyName string,
) error {
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, orgName, projectName, agentName, envID)
	if err != nil {
		return err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return err
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastRevokeToGateways(gateways, artifactUUID, artifactUUID, keyName)
}

// RotateAPIKey generates a new API key value and broadcasts the update to the environment's gateways.
// Returns the new API key (shown once) and its identifier.
func (s *AgentAPIKeyService) RotateAPIKey(
	ctx context.Context,
	orgName, projectName, agentName, envID, keyName string,
	req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, orgName, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return nil, err
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastRotateToGateways(gateways, orgName, artifactUUID, artifactUUID, keyName, req)
}

// ListAPIKeys returns API keys for the given agent (masked values only).
func (s *AgentAPIKeyService) ListAPIKeys(
	ctx context.Context,
	orgName, projectName, agentName, envID string,
) ([]models.StoredAPIKey, error) {
	artifact, _, err := s.resolveAgentAPIArtifact(ctx, orgName, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	all, err := s.apiKeyRepo.ListPermanentByArtifactKind(orgName, models.KindAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	var result []models.StoredAPIKey
	for _, k := range all {
		if k.ArtifactUUID == artifact.UUID {
			result = append(result, k)
		}
	}
	return result, nil
}

// IssueTestAPIKey issues (or rotates) the single short-lived test API key
// associated with an agent. Used by the console Try-It flow. The key is
// scoped by APIKeyTestKeyName and never appears in the user-facing list.
func (s *AgentAPIKeyService) IssueTestAPIKey(
	ctx context.Context,
	orgName, projectName, agentName, envID string,
) (*models.IssueTestAPIKeyResponse, error) {
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, orgName, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return nil, err
	}
	artifactUUID := artifact.UUID.String()

	expiresAt := time.Now().UTC().Add(testKeyTTL).Format(time.RFC3339)

	existing, err := s.apiKeyRepo.GetByArtifactAndName(artifactUUID, models.APIKeyTestKeyName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to look up existing test key: %w", err)
	}

	var resp *models.CreateAPIKeyResponse
	if existing != nil {
		if existing.Purpose != models.APIKeyPurposeTest {
			return nil, fmt.Errorf("%w: %q is reserved for console test keys", utils.ErrBadRequest, models.APIKeyTestKeyName)
		}
		// Same DB row, new hash + expiry; purpose is preserved (Upsert.DoUpdates excludes it).
		resp, err = s.broadcaster.broadcastRotateToGateways(gateways, orgName, artifactUUID, artifactUUID, models.APIKeyTestKeyName,
			&models.RotateAPIKeyRequest{ExpiresAt: &expiresAt})
	} else {
		resp, err = s.broadcaster.broadcastCreateToGateways(gateways, orgName, artifactUUID, artifactUUID,
			&models.CreateAPIKeyRequest{
				Name:        models.APIKeyTestKeyName,
				DisplayName: "Console Try-It",
				Purpose:     models.APIKeyPurposeTest,
				ExpiresAt:   &expiresAt,
			})
	}
	if err != nil {
		return nil, err
	}

	return &models.IssueTestAPIKeyResponse{
		Status:    resp.Status,
		Message:   resp.Message,
		KeyID:     resp.KeyID,
		APIKey:    resp.APIKey,
		ExpiresAt: expiresAt,
	}, nil
}
