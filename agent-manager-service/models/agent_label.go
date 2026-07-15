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

package models

import (
	"time"

	"github.com/google/uuid"
)

// AgentLabel is the sidecar record holding user-defined labels for an agent.
// Agent records themselves live in the OpenChoreo control plane, so labels are
// keyed by (ou_id, project_name, agent_name) like the other per-agent tables.
type AgentLabel struct {
	ID          uuid.UUID         `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID        string            `gorm:"column:ou_id"`
	ProjectName string            `gorm:"column:project_name"`
	AgentName   string            `gorm:"column:agent_name"`
	Labels      map[string]string `gorm:"column:labels;type:jsonb;serializer:json"`
	CreatedAt   time.Time         `gorm:"column:created_at"`
	UpdatedAt   time.Time         `gorm:"column:updated_at"`
}

func (AgentLabel) TableName() string { return "agent_labels" }
