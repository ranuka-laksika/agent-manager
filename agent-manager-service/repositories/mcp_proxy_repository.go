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
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyRepository defines the interface for MCP proxy persistence.
type MCPProxyRepository interface {
	Create(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, handle, name, version, orgUUID string) error
	Update(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, orgUUID string) error
	GetByUUID(ctx context.Context, proxyUUID, orgUUID string) (*models.MCPProxy, error)
	GetByHandle(ctx context.Context, handle, orgUUID string) (*models.MCPProxy, error)
	GetByHandleForUpdate(ctx context.Context, tx *gorm.DB, handle, orgUUID string) (*models.MCPProxy, error)
	List(ctx context.Context, orgUUID string, limit, offset int) ([]*models.MCPProxy, error)
	Delete(ctx context.Context, handle, orgUUID string) error
	Count(ctx context.Context, orgUUID string) (int, error)
	Exists(ctx context.Context, handle, orgUUID string) (bool, error)
}

// MCPProxyRepo implements MCPProxyRepository using GORM.
type MCPProxyRepo struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
}

// NewMCPProxyRepo creates a new MCP proxy repository.
func NewMCPProxyRepo(db *gorm.DB) MCPProxyRepository {
	return &MCPProxyRepo{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
	}
}

// Create inserts a new MCP proxy and its artifact row.
func (r *MCPProxyRepo) Create(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, handle, name, version, orgUUID string) error {
	if p.UUID == uuid.Nil {
		p.UUID = uuid.New()
	}
	now := time.Now()

	if err := r.artifactRepo.Create(tx.WithContext(ctx), &models.Artifact{
		UUID:             p.UUID,
		Handle:           handle,
		Name:             name,
		Version:          version,
		Kind:             models.KindMCPProxy,
		OrganizationName: orgUUID,
		CreatedAt:        now,
		UpdatedAt:        now,
		InCatalog:        true,
	}); err != nil {
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	if err := tx.WithContext(ctx).Create(p).Error; err != nil {
		return err
	}
	return nil
}

// Update modifies an existing MCP proxy and bumps the backing artifact timestamp.
func (r *MCPProxyRepo) Update(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, orgUUID string) error {
	if p == nil || p.UUID == uuid.Nil {
		return gorm.ErrRecordNotFound
	}

	result := tx.WithContext(ctx).Model(&models.MCPProxy{}).
		Where("uuid = ?", p.UUID).
		Updates(map[string]interface{}{
			"description":   p.Description,
			"status":        p.Status,
			"configuration": p.Configuration,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	return r.artifactRepo.Update(tx.WithContext(ctx), &models.Artifact{
		UUID:             p.UUID,
		Name:             p.Configuration.Name,
		Version:          p.Configuration.Version,
		OrganizationName: orgUUID,
	})
}

// GetByUUID retrieves an MCP proxy by artifact UUID.
func (r *MCPProxyRepo) GetByUUID(ctx context.Context, proxyUUID, orgUUID string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("mcp_proxies.uuid = ? AND a.organization_name = ? AND a.kind = ?", proxyUUID, orgUUID, models.KindMCPProxy).
		First(&proxy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &proxy, nil
}

// GetByHandle retrieves an MCP proxy by artifact handle.
func (r *MCPProxyRepo) GetByHandle(ctx context.Context, handle, orgUUID string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.organization_name = ? AND a.kind = ?", handle, orgUUID, models.KindMCPProxy).
		First(&proxy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &proxy, nil
}

// GetByHandleForUpdate retrieves an MCP proxy by handle and locks the
// underlying mcp_proxies and artifacts rows for the duration of the
// transaction. Use this to serialize read-modify-write flows against
// concurrent updates.
func (r *MCPProxyRepo) GetByHandleForUpdate(ctx context.Context, tx *gorm.DB, handle, orgUUID string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Artifact").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.organization_name = ? AND a.kind = ?", handle, orgUUID, models.KindMCPProxy).
		First(&proxy).Error
	if err != nil {
		return nil, err
	}
	return &proxy, nil
}

// List retrieves MCP proxies with pagination.
func (r *MCPProxyRepo) List(ctx context.Context, orgUUID string, limit, offset int) ([]*models.MCPProxy, error) {
	var proxies []*models.MCPProxy
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.organization_name = ? AND a.kind = ?", orgUUID, models.KindMCPProxy).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&proxies).Error
	if err != nil {
		return proxies, err
	}

	return proxies, nil
}

// Delete removes an MCP proxy and its artifact row.
func (r *MCPProxyRepo) Delete(ctx context.Context, handle, orgUUID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var proxy models.MCPProxy
		err := tx.
			Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
			Where("a.handle = ? AND a.organization_name = ? AND a.kind = ?", handle, orgUUID, models.KindMCPProxy).
			First(&proxy).Error
		if err != nil {
			return err
		}

		if err := tx.Where("artifact_uuid = ? AND organization_name = ?", proxy.UUID, orgUUID).
			Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND organization_name = ?", proxy.UUID, orgUUID).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("uuid = ?", proxy.UUID).Delete(&models.MCPProxy{}).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return utils.ErrMCPProxyHasMappings
			}
			return err
		}
		return r.artifactRepo.Delete(tx, proxy.UUID.String())
	})
}

// Count counts MCP proxies for an organization.
func (r *MCPProxyRepo) Count(ctx context.Context, orgUUID string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.MCPProxy{}).
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.organization_name = ? AND a.kind = ?", orgUUID, models.KindMCPProxy).
		Count(&count).Error
	return int(count), err
}

// Exists checks whether an MCP proxy exists by handle and organization.
func (r *MCPProxyRepo) Exists(ctx context.Context, handle, orgUUID string) (bool, error) {
	return r.artifactRepo.Exists(models.KindMCPProxy, handle, orgUUID)
}
