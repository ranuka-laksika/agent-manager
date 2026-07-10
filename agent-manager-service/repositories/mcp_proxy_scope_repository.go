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

package repositories

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MCPProxyScopeRepository persists per-proxy MCP scopes.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/mcp_proxy_scope_repository_mock.go . MCPProxyScopeRepository:MCPProxyScopeRepositoryMock
type MCPProxyScopeRepository interface {
	// ListByProxy returns the proxy's scopes, action-ascending.
	ListByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyScope, error)
	// ListByProxyUUIDs returns scopes for all given proxies (aggregate endpoint).
	ListByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]models.MCPProxyScope, error)
	// Get returns gorm.ErrRecordNotFound (unwrapped) when absent.
	Get(ctx context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error)
	Create(ctx context.Context, scope *models.MCPProxyScope) error
	Update(ctx context.Context, scope *models.MCPProxyScope) error
	Delete(ctx context.Context, proxyUUID uuid.UUID, action string) error
}

type mcpProxyScopeRepository struct{ db *gorm.DB }

// NewMCPProxyScopeRepository creates a new MCP proxy scope repository.
func NewMCPProxyScopeRepository(db *gorm.DB) MCPProxyScopeRepository {
	return &mcpProxyScopeRepository{db: db}
}

func (r *mcpProxyScopeRepository) ListByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyScope, error) {
	var scopes []models.MCPProxyScope
	if err := r.db.WithContext(ctx).Where("mcp_proxy_uuid = ?", proxyUUID).
		Order("action asc").Find(&scopes).Error; err != nil {
		return nil, fmt.Errorf("failed to list mcp proxy scopes: %w", err)
	}
	return scopes, nil
}

func (r *mcpProxyScopeRepository) ListByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]models.MCPProxyScope, error) {
	if len(proxyUUIDs) == 0 {
		return nil, nil
	}
	var scopes []models.MCPProxyScope
	if err := r.db.WithContext(ctx).Where("mcp_proxy_uuid IN ?", proxyUUIDs).
		Find(&scopes).Error; err != nil {
		return nil, fmt.Errorf("failed to list mcp proxy scopes: %w", err)
	}
	return scopes, nil
}

func (r *mcpProxyScopeRepository) Get(ctx context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error) {
	var scope models.MCPProxyScope
	if err := r.db.WithContext(ctx).Where("mcp_proxy_uuid = ? AND action = ?", proxyUUID, action).
		First(&scope).Error; err != nil {
		return nil, err
	}
	return &scope, nil
}

func (r *mcpProxyScopeRepository) Create(ctx context.Context, scope *models.MCPProxyScope) error {
	if err := r.db.WithContext(ctx).Create(scope).Error; err != nil {
		return fmt.Errorf("failed to create mcp proxy scope: %w", err)
	}
	return nil
}

func (r *mcpProxyScopeRepository) Update(ctx context.Context, scope *models.MCPProxyScope) error {
	res := r.db.WithContext(ctx).Model(&models.MCPProxyScope{}).
		Where("mcp_proxy_uuid = ? AND action = ?", scope.MCPProxyUUID, scope.Action).
		Updates(map[string]any{"description": scope.Description, "tools": scope.Tools, "updated_at": time.Now()})
	if res.Error != nil {
		return fmt.Errorf("failed to update mcp proxy scope: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *mcpProxyScopeRepository) Delete(ctx context.Context, proxyUUID uuid.UUID, action string) error {
	res := r.db.WithContext(ctx).Where("mcp_proxy_uuid = ? AND action = ?", proxyUUID, action).
		Delete(&models.MCPProxyScope{})
	if res.Error != nil {
		return fmt.Errorf("failed to delete mcp proxy scope: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
