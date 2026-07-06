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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// testKeyTTL is the validity window for a console-issued test API key.
// The console refreshes the key at staleTime well before this elapses.
const testKeyTTL = 10 * time.Minute

// legacyTestKeyName is the pre-per-user shared test key name; still reserved
// so user-created keys can't collide with lingering legacy rows.
const legacyTestKeyName = "console-test"

// testKeyName returns a stable, per-user key name derived from the JWT subject.
// Hashing keeps the name within the gateway's alphanumeric-hyphen constraint
// and avoids storing raw user identifiers as key names.
func testKeyName(userSub string) string {
	h := sha256.Sum256([]byte(userSub))
	return models.APIKeyTestKeyPrefix + hex.EncodeToString(h[:])[:12]
}

// AgentAPIKeyServiceInterface defines the contract for agent API key operations
type AgentAPIKeyServiceInterface interface {
	CreateAPIKey(ctx context.Context, ouID, projectName, agentName, envID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	RevokeAPIKey(ctx context.Context, ouID, projectName, agentName, envID, keyName string) error
	RotateAPIKey(ctx context.Context, ouID, projectName, agentName, envID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	ListAPIKeys(ctx context.Context, ouID, projectName, agentName, envID string) ([]models.StoredAPIKey, error)
	IssueTestAPIKey(ctx context.Context, ouID, projectName, agentName, envID, userSub string) (*models.IssueTestAPIKeyResponse, error)
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

func (s *AgentAPIKeyService) resolveAgentAPIArtifact(ctx context.Context, ouID, projectName, agentName, envID string) (*models.Artifact, string, error) {
	environment, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get environment: %w", translateEnvironmentError(err))
	}

	artifact, err := s.artifactRepo.GetByHandle(agentEnvAPIArtifactHandle(projectName, agentName, environment.UUID), ouID)
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
	ouID, projectName, agentName, envID string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if req != nil && (strings.HasPrefix(req.Name, models.APIKeyTestKeyPrefix) || req.Name == legacyTestKeyName) {
		return nil, fmt.Errorf("%w: names starting with %q are reserved for console test keys", utils.ErrBadRequest, models.APIKeyTestKeyPrefix)
	}
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, ouID, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return nil, err
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastCreateToGateways(gateways, ouID, artifactUUID, artifactUUID, req)
}

// RevokeAPIKey broadcasts an API key revocation event to the environment's gateways.
func (s *AgentAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	ouID, projectName, agentName, envID, keyName string,
) error {
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, ouID, projectName, agentName, envID)
	if err != nil {
		return err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return err
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastRevokeToGateways(ctx, gateways, artifactUUID, artifactUUID, keyName)
}

// RotateAPIKey generates a new API key value and broadcasts the update to the environment's gateways.
// Returns the new API key (shown once) and its identifier.
func (s *AgentAPIKeyService) RotateAPIKey(
	ctx context.Context,
	ouID, projectName, agentName, envID, keyName string,
	req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, ouID, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return nil, err
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastRotateToGateways(gateways, ouID, artifactUUID, artifactUUID, keyName, req)
}

// ListAPIKeys returns API keys for the given agent (masked values only).
func (s *AgentAPIKeyService) ListAPIKeys(
	ctx context.Context,
	ouID, projectName, agentName, envID string,
) ([]models.StoredAPIKey, error) {
	artifact, _, err := s.resolveAgentAPIArtifact(ctx, ouID, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	all, err := s.apiKeyRepo.ListPermanentByArtifactKind(ouID, models.KindAgent)
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

// IssueTestAPIKey issues (or rotates) a short-lived test API key scoped to the
// calling user (userSub). Each user gets their own DB row, so concurrent sessions
// across different users don't invalidate each other's keys. Used by the console
// Try-It flow; test keys never appear in the user-facing list.
func (s *AgentAPIKeyService) IssueTestAPIKey(
	ctx context.Context,
	ouID, projectName, agentName, envID, userSub string,
) (*models.IssueTestAPIKeyResponse, error) {
	artifact, envUUID, err := s.resolveAgentAPIArtifact(ctx, ouID, projectName, agentName, envID)
	if err != nil {
		return nil, err
	}
	gateways, err := s.resolveEnvGateways(envUUID)
	if err != nil {
		return nil, err
	}
	artifactUUID := artifact.UUID.String()

	keyName := testKeyName(userSub)
	expiresAt := time.Now().UTC().Add(testKeyTTL).Format(time.RFC3339)

	existing, err := s.apiKeyRepo.GetByArtifactAndName(artifactUUID, keyName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to look up existing test key: %w", err)
	}

	log := logger.GetLogger(ctx)
	var resp *models.CreateAPIKeyResponse
	if existing != nil {
		if existing.Purpose != models.APIKeyPurposeTest {
			return nil, fmt.Errorf("%w: %q is reserved for console test keys", utils.ErrBadRequest, keyName)
		}
		// Same DB row, new hash + expiry; purpose is preserved (Upsert.DoUpdates excludes it).
		resp, err = s.broadcaster.broadcastRotateToGateways(gateways, ouID, artifactUUID, artifactUUID, keyName,
			&models.RotateAPIKeyRequest{ExpiresAt: &expiresAt})
		if err == nil {
			log.Info("IssueTestAPIKey: rotated", "keyName", keyName, "org", ouID, "agent", agentName, "env", envID, "expiresAt", expiresAt)
		}
	} else {
		resp, err = s.broadcaster.broadcastCreateToGateways(gateways, ouID, artifactUUID, artifactUUID,
			&models.CreateAPIKeyRequest{
				Name:        keyName,
				DisplayName: "Console Try-It",
				Purpose:     models.APIKeyPurposeTest,
				ExpiresAt:   &expiresAt,
			})
		if err == nil {
			log.Info("IssueTestAPIKey: created", "keyName", keyName, "org", ouID, "agent", agentName, "env", envID, "expiresAt", expiresAt)
		}
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
