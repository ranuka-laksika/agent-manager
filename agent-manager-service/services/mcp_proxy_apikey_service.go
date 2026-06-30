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
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// MCPProxyAPIKeyService handles API key management for source MCP proxies.
type MCPProxyAPIKeyService struct {
	proxyRepo   repositories.MCPProxyRepository
	apiKeyRepo  repositories.APIKeyRepository
	broadcaster apiKeyBroadcaster
}

// NewMCPProxyAPIKeyService creates a new MCP proxy API key service instance.
func NewMCPProxyAPIKeyService(
	proxyRepo repositories.MCPProxyRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
) *MCPProxyAPIKeyService {
	return &MCPProxyAPIKeyService{
		proxyRepo:  proxyRepo,
		apiKeyRepo: apiKeyRepo,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
			apiKeyRepo:     apiKeyRepo,
		},
	}
}

// getMCPProxyByID resolves a source MCP proxy by its handle within the org.
func (s *MCPProxyAPIKeyService) getMCPProxyByID(ctx context.Context, orgUUID, proxyID string) (*models.MCPProxy, error) {
	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return nil, utils.ErrInvalidInput
	}
	proxy, err := s.proxyRepo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to get MCP proxy: %w", err)
	}
	return proxy, nil
}

// ListAPIKeys returns the masked, user-managed (permanent) API keys for an MCP proxy.
// The plain key value is never returned — only the masked representation.
func (s *MCPProxyAPIKeyService) ListAPIKeys(ctx context.Context, orgUUID, proxyID string) (*models.ListAPIKeysResponse, error) {
	proxy, err := s.getMCPProxyByID(ctx, orgUUID, proxyID)
	if err != nil {
		return nil, err
	}

	stored, err := s.apiKeyRepo.ListByArtifact(ctx, proxy.UUID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return &models.ListAPIKeysResponse{Keys: models.ToUserManagedAPIKeyInfos(stored)}, nil
}

// CreateAPIKey generates an API key for a source MCP proxy and broadcasts it to all gateways.
func (s *MCPProxyAPIKeyService) CreateAPIKey(ctx context.Context, orgUUID, proxyID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	proxy, err := s.getMCPProxyByID(ctx, orgUUID, proxyID)
	if err != nil {
		return nil, err
	}
	proxyUUID := proxy.UUID.String()
	return s.broadcaster.broadcastCreate(ctx, orgUUID, proxyUUID, proxyUUID, req)
}

// RevokeAPIKey revokes an API key for a source MCP proxy and broadcasts the revocation.
func (s *MCPProxyAPIKeyService) RevokeAPIKey(ctx context.Context, orgUUID, proxyID, keyName string) error {
	proxy, err := s.getMCPProxyByID(ctx, orgUUID, proxyID)
	if err != nil {
		return err
	}
	proxyUUID := proxy.UUID.String()
	return s.broadcaster.broadcastRevoke(ctx, orgUUID, proxyUUID, proxyUUID, keyName)
}

// RotateAPIKey rotates an API key for a source MCP proxy and broadcasts the new hash.
func (s *MCPProxyAPIKeyService) RotateAPIKey(ctx context.Context, orgUUID, proxyID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	proxy, err := s.getMCPProxyByID(ctx, orgUUID, proxyID)
	if err != nil {
		return nil, err
	}
	proxyUUID := proxy.UUID.String()
	return s.broadcaster.broadcastRotate(ctx, orgUUID, proxyUUID, proxyUUID, keyName, req)
}
