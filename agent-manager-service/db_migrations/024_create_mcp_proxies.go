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

var migration024 = migration{
	ID: 24,
	Migrate: func(db *gorm.DB) error {
		sql := `
			CREATE TABLE mcp_proxies (
				uuid UUID PRIMARY KEY,
				description TEXT,
				created_by VARCHAR(255),
				status VARCHAR(20) NOT NULL DEFAULT 'pending',
				configuration JSONB NOT NULL,

				CONSTRAINT fk_mcp_proxy_artifact FOREIGN KEY (uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE
			);
			CREATE TABLE mcp_proxy_mappings (
				uuid UUID PRIMARY KEY,
				source_mcp_proxy_uuid UUID NOT NULL,
				description TEXT,
				status VARCHAR(20) NOT NULL DEFAULT 'pending',
				configuration JSONB NOT NULL,

				CONSTRAINT fk_mcp_proxy_mapping_artifact FOREIGN KEY (uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_mcp_proxy_mapping_source FOREIGN KEY (source_mcp_proxy_uuid)
					REFERENCES mcp_proxies(uuid) ON DELETE RESTRICT
			);
			CREATE TABLE env_agent_mcp_mapping (
				id SERIAL PRIMARY KEY,
				config_uuid UUID NOT NULL,
				environment_uuid UUID NOT NULL,
				mcp_proxy_uuid UUID NOT NULL,
				artifact_uuid UUID NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_env_mcp_mapping_config FOREIGN KEY (config_uuid)
					REFERENCES agent_configurations(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_env_mcp_mapping_proxy FOREIGN KEY (mcp_proxy_uuid)
					REFERENCES mcp_proxies(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_env_mcp_mapping_artifact FOREIGN KEY (artifact_uuid)
					REFERENCES mcp_proxy_mappings(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_env_mcp_mapping UNIQUE(config_uuid, environment_uuid)
			);
			CREATE INDEX IF NOT EXISTS idx_mcp_proxy_mapping_source ON mcp_proxy_mappings(source_mcp_proxy_uuid);
			CREATE INDEX IF NOT EXISTS idx_env_mcp_mapping_config ON env_agent_mcp_mapping(config_uuid);
			CREATE INDEX IF NOT EXISTS idx_env_mcp_mapping_environment ON env_agent_mcp_mapping(environment_uuid);
			CREATE INDEX IF NOT EXISTS idx_env_mcp_mapping_proxy ON env_agent_mcp_mapping(mcp_proxy_uuid);
			CREATE INDEX IF NOT EXISTS idx_env_mcp_mapping_artifact ON env_agent_mcp_mapping(artifact_uuid);
			CREATE INDEX IF NOT EXISTS idx_env_mcp_mapping_config_env ON env_agent_mcp_mapping(config_uuid, environment_uuid);

			-- Gateways store their latest reported policy manifest here;
			-- consumed by policy listings.
			ALTER TABLE gateways ADD COLUMN IF NOT EXISTS manifest JSONB;
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}
