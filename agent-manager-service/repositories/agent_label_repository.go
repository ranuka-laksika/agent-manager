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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrAgentLabelsNotFound is returned when no labels row exists for the agent.
var ErrAgentLabelsNotFound = errors.New("agent labels not found")

// AgentLabelRepository persists user-defined labels for agents, which live in
// the OpenChoreo control plane and therefore have no canonical local table.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/agent_label_repository_mock.go . AgentLabelRepository:AgentLabelRepositoryMock
type AgentLabelRepository interface {
	// Upsert creates or replaces the labels row for an agent.
	Upsert(ctx context.Context, record *models.AgentLabel) error

	// Get retrieves the labels row for an agent, returning
	// ErrAgentLabelsNotFound when the agent has no labels.
	Get(ctx context.Context, ouID, projectName, agentName string) (*models.AgentLabel, error)

	// ListByProject retrieves all label rows for a project's agents.
	ListByProject(ctx context.Context, ouID, projectName string) ([]models.AgentLabel, error)

	// DeleteAllByAgent removes the labels row for an agent (used when the agent is deleted).
	DeleteAllByAgent(ctx context.Context, ouID, projectName, agentName string) error
}

// AgentLabelRepo implements AgentLabelRepository using GORM.
type AgentLabelRepo struct {
	db *gorm.DB
}

// NewAgentLabelRepo creates a new agent label repository.
func NewAgentLabelRepo(db *gorm.DB) AgentLabelRepository {
	return &AgentLabelRepo{db: db}
}

// Upsert creates or replaces the labels row for an agent.
func (r *AgentLabelRepo) Upsert(ctx context.Context, record *models.AgentLabel) error {
	// clause.Assignments bypasses GORM's serializer:json tag, so the labels map
	// must be pre-serialized and cast to jsonb explicitly. A nil map marshals
	// to SQL null — normalize first (the column is NOT NULL).
	if record.Labels == nil {
		record.Labels = map[string]string{}
	}
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	labelsJSON, err := json.Marshal(record.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal agent labels: %w", err)
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "ou_id"}, {Name: "project_name"}, {Name: "agent_name"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"labels":     clause.Expr{SQL: "?::jsonb", Vars: []interface{}{string(labelsJSON)}},
			"updated_at": clause.Expr{SQL: "NOW()"},
		}),
	}).Create(record).Error
}

// Get retrieves the labels row for an agent.
func (r *AgentLabelRepo) Get(ctx context.Context, ouID, projectName, agentName string) (*models.AgentLabel, error) {
	var record models.AgentLabel
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentLabelsNotFound
		}
		return nil, fmt.Errorf("failed to get agent labels: %w", err)
	}
	return &record, nil
}

// ListByProject retrieves all label rows for a project's agents.
func (r *AgentLabelRepo) ListByProject(ctx context.Context, ouID, projectName string) ([]models.AgentLabel, error) {
	var records []models.AgentLabel
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ?", ouID, projectName).
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list agent labels: %w", err)
	}
	return records, nil
}

// DeleteAllByAgent removes the labels row for an agent.
func (r *AgentLabelRepo) DeleteAllByAgent(ctx context.Context, ouID, projectName, agentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.AgentLabel{}).Error
}
