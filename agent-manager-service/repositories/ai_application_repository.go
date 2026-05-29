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

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// AIApplicationRepository defines the interface for AI application persistence.
type AIApplicationRepository interface {
	// Create inserts an AIApplication row. Uses ON CONFLICT DO NOTHING so it is safe to
	// call idempotently on retry. The app UUID is populated if the row was inserted; it
	// remains zero if the conflict path fired (caller must re-fetch with GetByAgentEnv).
	Create(ctx context.Context, tx *gorm.DB, app *models.AIApplication) error
	// GetByAgentEnv returns the AIApplication for the given org/project/agent/environment.
	// Returns gorm.ErrRecordNotFound when no row exists.
	GetByAgentEnv(ctx context.Context, orgName, projectName, agentID, envName string) (*models.AIApplication, error)
	// ListByOrg returns all AIApplication rows for the given organisation.
	ListByOrg(ctx context.Context, orgName string) ([]models.AIApplication, error)
	// DeleteByAgentEnv removes the AIApplication row for the given agent+environment. No-op if absent.
	DeleteByAgentEnv(ctx context.Context, tx *gorm.DB, orgName, projectName, agentID, envName string) error
}

// AIApplicationRepo implements AIApplicationRepository using GORM.
type AIApplicationRepo struct {
	db *gorm.DB
}

// NewAIApplicationRepository creates a new AIApplicationRepo.
func NewAIApplicationRepository(db *gorm.DB) *AIApplicationRepo {
	return &AIApplicationRepo{db: db}
}

// Create inserts an AIApplication, ignoring conflicts on (organization_name, project_name, agent_id, environment_name).
func (r *AIApplicationRepo) Create(ctx context.Context, tx *gorm.DB, app *models.AIApplication) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(app).Error
}

// GetByAgentEnv fetches the AIApplication for a specific agent+environment in an org.
func (r *AIApplicationRepo) GetByAgentEnv(ctx context.Context, orgName, projectName, agentID, envName string) (*models.AIApplication, error) {
	var app models.AIApplication
	err := r.db.WithContext(ctx).
		Where("organization_name = ? AND project_name = ? AND agent_id = ? AND environment_name = ?",
			orgName, projectName, agentID, envName).
		First(&app).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// ListByOrg returns all AIApplication rows for an organisation.
func (r *AIApplicationRepo) ListByOrg(ctx context.Context, orgName string) ([]models.AIApplication, error) {
	var apps []models.AIApplication
	err := r.db.WithContext(ctx).
		Where("organization_name = ?", orgName).
		Find(&apps).Error
	return apps, err
}

// DeleteByAgentEnv removes the AIApplication row for the given agent+environment. No-op if absent.
func (r *AIApplicationRepo) DeleteByAgentEnv(ctx context.Context, tx *gorm.DB, orgName, projectName, agentID, envName string) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.WithContext(ctx).
		Where("organization_name = ? AND project_name = ? AND agent_id = ? AND environment_name = ?",
			orgName, projectName, agentID, envName).
		Delete(&models.AIApplication{}).Error
}
