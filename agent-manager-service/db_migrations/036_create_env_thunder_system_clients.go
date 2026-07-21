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

package dbmigrations

import (
	"gorm.io/gorm"
)

// env_thunder_system_clients stores each environment's env-Thunder system-client
// secret encrypted, so AMS decrypts from Postgres instead of a key vault.
//
// Keyed by (ou_id, env_name), not org_name: ou_id is the stable, multi-tenant-safe
// identity (org_name/handles aren't guaranteed unique once real multi-tenant SaaS
// arrives). No org_name column: Thunder namespace/URL building deliberately
// stays pinned to ThunderOrgNamespace() (config, not this table — see its own
// doc comment), so org_name would just be write-only data with no reader.
var migration036 = migration{
	ID: 36,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			createTable := `
			CREATE TABLE IF NOT EXISTS env_thunder_system_clients (
				id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id                   VARCHAR(255) NOT NULL,
				env_name                VARCHAR(255) NOT NULL,
				client_id               VARCHAR(255) NOT NULL DEFAULT 'amp-system-client',
				client_secret_encrypted BYTEA NOT NULL,
				created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

				CONSTRAINT uq_env_thunder_system_clients UNIQUE (ou_id, env_name)
			)`

			// No separate index on (ou_id, env_name): the UNIQUE constraint
			return runSQL(tx, createTable)
		})
	},
}
