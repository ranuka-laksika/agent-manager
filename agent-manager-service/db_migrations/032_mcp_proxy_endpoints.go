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

import "gorm.io/gorm"

// migration032 re-models MCP proxy per-environment configuration as endpoints.
//
// An MCP proxy (mcp_proxies) becomes a logical grouping. Each endpoint
// (mcp_proxy_endpoints) is the deployable proxy definition (upstream + auth,
// policies, capabilities, security), and one endpoint can be deployed to 1..N
// environments through mcp_proxy_endpoint_environments, which holds the stable
// per-deployment gateway artifact identity and status.
//
// The hard rule "within one proxy an environment maps to at most one endpoint"
// is enforced by uq_proxy_env_single. Because Postgres cannot reach through the
// endpoint FK to enforce this, mcp_proxy_uuid is denormalized onto the join row.
//
// This is a fresh schema (pre-GA): the old mcp_proxies.configuration.environments
// JSONB blob is abandoned, not migrated. No DDL is needed to drop it since the
// column is JSONB and the application simply stops reading/writing that key.
var migration032 = migration{
	ID: 32,
	Migrate: func(db *gorm.DB) error {
		sql := `
			CREATE TABLE mcp_proxy_endpoints (
				uuid UUID PRIMARY KEY,
				mcp_proxy_uuid UUID NOT NULL,
				handle VARCHAR(255) NOT NULL,
				name VARCHAR(255),
				status VARCHAR(20) NOT NULL DEFAULT 'pending',
				configuration JSONB NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_mcp_endpoint_proxy FOREIGN KEY (mcp_proxy_uuid)
					REFERENCES mcp_proxies(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_mcp_endpoint_handle UNIQUE(mcp_proxy_uuid, handle)
			);
			CREATE TABLE mcp_proxy_endpoint_environments (
				id SERIAL PRIMARY KEY,
				mcp_proxy_uuid UUID NOT NULL,
				endpoint_uuid UUID NOT NULL,
				environment_uuid UUID NOT NULL,
				artifact_uuid UUID NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'pending',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_endpoint_env_endpoint FOREIGN KEY (endpoint_uuid)
					REFERENCES mcp_proxy_endpoints(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_endpoint_env_proxy FOREIGN KEY (mcp_proxy_uuid)
					REFERENCES mcp_proxies(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_endpoint_env_artifact FOREIGN KEY (artifact_uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_endpoint_env UNIQUE(endpoint_uuid, environment_uuid),
				CONSTRAINT uq_proxy_env_single UNIQUE(mcp_proxy_uuid, environment_uuid)
			);
			-- Indexes are only created for columns not already covered by a unique
			-- constraint's leading column: uq_mcp_endpoint_handle(mcp_proxy_uuid, handle),
			-- uq_endpoint_env(endpoint_uuid, environment_uuid) and
			-- uq_proxy_env_single(mcp_proxy_uuid, environment_uuid) already index
			-- mcp_proxy_uuid and endpoint_uuid, so only environment_uuid and artifact_uuid
			-- need their own index.
			CREATE INDEX IF NOT EXISTS idx_endpoint_env_environment ON mcp_proxy_endpoint_environments(environment_uuid);
			CREATE INDEX IF NOT EXISTS idx_endpoint_env_artifact ON mcp_proxy_endpoint_environments(artifact_uuid);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}
