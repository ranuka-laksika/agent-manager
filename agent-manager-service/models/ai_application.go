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

// AIApplication tracks a per-agent-per-environment application record used for
// consumer-level rate limiting. One application is shared across all LLM configs
// of the same agent in the same environment; all proxy API keys for that agent+env
// are bound to it so the gateway policy engine can apply per-consumer limits.
type AIApplication struct {
	UUID             uuid.UUID `gorm:"column:uuid;primaryKey;type:uuid;default:gen_random_uuid()" json:"uuid"`
	Handle           string    `gorm:"column:handle;not null" json:"handle"`
	Name             string    `gorm:"column:name;not null" json:"name"`
	AgentID          string    `gorm:"column:agent_id;not null" json:"agentId"`
	ProjectName      string    `gorm:"column:project_name;not null" json:"projectName"`
	EnvironmentName  string    `gorm:"column:environment_name;not null" json:"environmentName"`
	OrganizationName string    `gorm:"column:ou_id;not null" json:"organizationName"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the AIApplication model.
func (AIApplication) TableName() string {
	return "ai_applications"
}
