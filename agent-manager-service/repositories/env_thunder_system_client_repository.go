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

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// EnvThunderSystemClientRepository defines data access for per-environment
// env-Thunder system-client credentials.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/env_thunder_system_client_repository_mock.go . EnvThunderSystemClientRepository:EnvThunderSystemClientRepositoryMock
type EnvThunderSystemClientRepository interface {
	// Get returns the credential for (ouID, envName), or
	// (nil, gorm.ErrRecordNotFound) if none exists.
	Get(ctx context.Context, ouID, envName string) (*models.EnvThunderSystemClient, error)
	// Upsert atomically creates or updates the credential for its (OUID, EnvName).
	Upsert(ctx context.Context, cred *models.EnvThunderSystemClient) error
	// Delete removes the credential for (ouID, envName). Deleting a
	// non-existent row is not an error.
	Delete(ctx context.Context, ouID, envName string) error
}

type envThunderSystemClientRepo struct {
	db *gorm.DB
}

// NewEnvThunderSystemClientRepo creates a new EnvThunderSystemClientRepository.
func NewEnvThunderSystemClientRepo(db *gorm.DB) EnvThunderSystemClientRepository {
	return &envThunderSystemClientRepo{db: db}
}

func (r *envThunderSystemClientRepo) Get(ctx context.Context, ouID, envName string) (*models.EnvThunderSystemClient, error) {
	var cred models.EnvThunderSystemClient
	result := r.db.WithContext(ctx).Where("ou_id = ? AND env_name = ?", ouID, envName).First(&cred)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &cred, nil
}

func (r *envThunderSystemClientRepo) Upsert(ctx context.Context, cred *models.EnvThunderSystemClient) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ou_id"}, {Name: "env_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"client_id", "client_secret_encrypted", "updated_at"}),
	}).Create(cred).Error
}

func (r *envThunderSystemClientRepo) Delete(ctx context.Context, ouID, envName string) error {
	return r.db.WithContext(ctx).Where("ou_id = ? AND env_name = ?", ouID, envName).
		Delete(&models.EnvThunderSystemClient{}).Error
}
