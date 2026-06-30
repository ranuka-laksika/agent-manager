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
	"fmt"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// LLMProviderAPIKeyService handles API key management for LLM providers
type LLMProviderAPIKeyService struct {
	providerRepo repositories.LLMProviderRepository
	apiKeyRepo   repositories.APIKeyRepository
	broadcaster  apiKeyBroadcaster
}

// NewLLMProviderAPIKeyService creates a new LLM provider API key service instance
func NewLLMProviderAPIKeyService(
	providerRepo repositories.LLMProviderRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
) *LLMProviderAPIKeyService {
	return &LLMProviderAPIKeyService{
		providerRepo: providerRepo,
		apiKeyRepo:   apiKeyRepo,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
			apiKeyRepo:     apiKeyRepo,
		},
	}
}

// ListAPIKeys returns the masked, user-managed (permanent) API keys for an LLM provider.
// The plain key value is never returned — only the masked representation.
func (s *LLMProviderAPIKeyService) ListAPIKeys(
	ctx context.Context,
	orgID, providerID string,
) (*models.ListAPIKeysResponse, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	stored, err := s.apiKeyRepo.ListByArtifact(providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	keys := make([]models.APIKeyInfo, 0, len(stored))
	for _, k := range stored {
		// Only surface user-managed keys; hide console-managed and test keys.
		if k.Purpose != models.APIKeyPurposeUserManaged {
			continue
		}
		info := models.APIKeyInfo{
			Name:         k.Name,
			DisplayName:  k.DisplayName,
			MaskedAPIKey: k.MaskedAPIKey,
			Status:       k.Status,
			CreatedAt:    k.CreatedAt.Format(time.RFC3339),
		}
		if k.ExpiresAt != nil {
			expiresAt := k.ExpiresAt.Format(time.RFC3339)
			info.ExpiresAt = &expiresAt
		}
		keys = append(keys, info)
	}

	return &models.ListAPIKeysResponse{Keys: keys}, nil
}

// CreateAPIKey generates an API key for an LLM provider and broadcasts it to all gateways
func (s *LLMProviderAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgID, providerID string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}
	return s.broadcaster.broadcastCreate(orgID, providerID, providerID, req)
}

// RevokeAPIKey broadcasts an API key revocation event to all gateways for this organization.
func (s *LLMProviderAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	orgID, providerID, keyName string,
) error {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return utils.ErrLLMProviderNotFound
	}
	return s.broadcaster.broadcastRevoke(orgID, providerID, providerID, keyName)
}

// RevokeAllUserManagedKeys revokes every user-managed API key for an LLM provider and
// broadcasts the revocations to all gateways. Used when API key security is disabled.
func (s *LLMProviderAPIKeyService) RevokeAllUserManagedKeys(
	ctx context.Context,
	orgID, providerID string,
) error {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return utils.ErrLLMProviderNotFound
	}
	return s.broadcaster.broadcastRevokeUserManaged(orgID, providerID, providerID)
}

// RotateAPIKey generates a new API key value and broadcasts the update to all gateways.
// Returns the new API key (shown once) and its identifier.
func (s *LLMProviderAPIKeyService) RotateAPIKey(
	ctx context.Context,
	orgID, providerID, keyName string,
	req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}
	return s.broadcaster.broadcastRotate(orgID, providerID, providerID, keyName, req)
}
