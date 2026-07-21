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

// EnvThunderSystemClient is the encrypted env-Thunder system-client credential
// AMS uses to reach an environment's Thunder, keyed by (OUID, EnvName). No
// org_name column: Thunder namespace/URL building stays pinned to
// ThunderOrgNamespace() (config, not this table — see its own doc comment),
// so org_name would be write-only data with no reader.
type EnvThunderSystemClient struct {
	ID                    uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID                  string    `gorm:"column:ou_id;not null;uniqueIndex:uq_env_thunder_system_clients"`
	EnvName               string    `gorm:"column:env_name;not null;uniqueIndex:uq_env_thunder_system_clients"`
	ClientID              string    `gorm:"column:client_id;not null;default:'amp-system-client'"`
	ClientSecretEncrypted []byte    `gorm:"column:client_secret_encrypted;not null"`
	CreatedAt             time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt             time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

// TableName returns the table name for the EnvThunderSystemClient model.
func (EnvThunderSystemClient) TableName() string { return "env_thunder_system_clients" }
