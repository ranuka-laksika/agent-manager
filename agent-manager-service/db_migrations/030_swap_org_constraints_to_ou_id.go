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

// Makes ou_id the org-scoping key on all 15 org-scoped tables: every unique
// constraint, primary key, and index that included organization_name/org_name
// is recreated on ou_id, and the old org-name variants are dropped. The
// org-name columns themselves are kept as display data only (see 029, which
// adds ou_id and relaxes the org-name columns with a default).
//
// Note: this swap assumes ou_id is populated (fresh deployments, where the app
// writes ou_id from day one). It must not run against a database that still
// has rows with an empty ou_id.
var migration030 = migration{
	ID: 30,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(
				tx,
				// -- gateways (partial unique from 009 excludes soft-deleted rows;
				//    partial uniqueness is only expressible as an index) --
				`CREATE UNIQUE INDEX IF NOT EXISTS uq_gateway_ou_id_name_active
					ON gateways(ou_id, name)
					WHERE deleted_at IS NULL`,
				`DROP INDEX IF EXISTS uq_gateway_org_name_active`,
				`CREATE INDEX IF NOT EXISTS idx_gateways_ou_id ON gateways(ou_id)`,
				`DROP INDEX IF EXISTS idx_gateways_org`,

				// -- artifacts --
				`ALTER TABLE artifacts
					ADD CONSTRAINT uq_artifact_handle_ou_id UNIQUE(handle, ou_id),
					ADD CONSTRAINT uq_artifact_name_version_ou_id UNIQUE(name, version, ou_id),
					DROP CONSTRAINT IF EXISTS uq_artifact_handle_org,
					DROP CONSTRAINT IF EXISTS uq_artifact_name_version_org`,
				`CREATE INDEX IF NOT EXISTS idx_artifacts_ou_id ON artifacts(ou_id)`,
				`DROP INDEX IF EXISTS idx_artifacts_org`,

				// -- deployments --
				`CREATE INDEX IF NOT EXISTS idx_deployments_ou_id_gateway_created
					ON deployments(artifact_uuid, ou_id, gateway_uuid, created_at DESC)`,
				`DROP INDEX IF EXISTS idx_deployments_org_gateway_created`,

				// -- deployment_status (org column is part of the primary key) --
				`ALTER TABLE deployment_status
					DROP CONSTRAINT pk_deployment_status,
					ADD CONSTRAINT pk_deployment_status PRIMARY KEY (artifact_uuid, ou_id, gateway_uuid)`,

				// -- association_mappings --
				`ALTER TABLE association_mappings
					ADD CONSTRAINT uq_association_artifact_resource_type_ou_id
						UNIQUE(artifact_uuid, resource_uuid, association_type, ou_id),
					DROP CONSTRAINT IF EXISTS uq_association_artifact_resource_type`,
				`CREATE INDEX IF NOT EXISTS idx_association_artifact_resource_type_ou_id
					ON association_mappings(artifact_uuid, association_type, ou_id)`,
				`CREATE INDEX IF NOT EXISTS idx_association_resource_ou_id
					ON association_mappings(association_type, resource_uuid, ou_id)`,
				`CREATE INDEX IF NOT EXISTS idx_association_ou_id ON association_mappings(ou_id)`,
				`DROP INDEX IF EXISTS idx_association_artifact_resource_type`,
				`DROP INDEX IF EXISTS idx_association_resource`,
				`DROP INDEX IF EXISTS idx_association_org`,

				// -- llm_provider_templates --
				`ALTER TABLE llm_provider_templates
					ADD CONSTRAINT uq_llm_template_handle_ou_id_system UNIQUE(ou_id, handle, is_system),
					DROP CONSTRAINT IF EXISTS uq_llm_template_handle_org_system`,
				`CREATE INDEX IF NOT EXISTS idx_llm_provider_templates_ou_id ON llm_provider_templates(ou_id)`,
				`DROP INDEX IF EXISTS idx_llm_provider_templates_org`,

				// -- agent_configurations --
				`ALTER TABLE agent_configurations
					ADD CONSTRAINT uq_agent_config_name_ou_id UNIQUE(agent_id, name, ou_id, project_name),
					DROP CONSTRAINT IF EXISTS uq_agent_config_name`,
				`CREATE INDEX IF NOT EXISTS idx_agent_config_ou_id_project ON agent_configurations(ou_id, project_name)`,
				`CREATE INDEX IF NOT EXISTS idx_agent_config_ou_id_agent ON agent_configurations(ou_id, agent_id)`,
				`DROP INDEX IF EXISTS idx_agent_config_org_project`,
				`DROP INDEX IF EXISTS idx_agent_config_org_agent`,

				// -- api_keys (unique key is (artifact_uuid, name); only the org index moves) --
				`CREATE INDEX IF NOT EXISTS idx_api_keys_ou_id ON api_keys(ou_id)`,
				`DROP INDEX IF EXISTS idx_api_keys_org`,

				// -- ai_applications (unique constraint from 023 has an auto-generated
				//    name, so it is looked up in pg_constraint before dropping) --
				`DO $$
				DECLARE
					uq_name text;
				BEGIN
					SELECT c.conname INTO uq_name
					FROM pg_constraint c
					WHERE c.conrelid = 'ai_applications'::regclass
					  AND c.contype = 'u';
					IF uq_name IS NOT NULL THEN
						EXECUTE format('ALTER TABLE ai_applications DROP CONSTRAINT %I', uq_name);
					END IF;
				END $$`,
				`ALTER TABLE ai_applications
					ADD CONSTRAINT uq_ai_applications_ou_id_project_agent_env
						UNIQUE(ou_id, project_name, agent_id, environment_name)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_applications_agent_ou_id
					ON ai_applications(ou_id, project_name, agent_id)`,
				`CREATE INDEX IF NOT EXISTS idx_ai_applications_ou_id ON ai_applications(ou_id)`,
				`DROP INDEX IF EXISTS idx_ai_applications_agent`,
				`DROP INDEX IF EXISTS idx_ai_applications_org`,

				// -- monitors --
				`ALTER TABLE monitors
					ADD CONSTRAINT uq_monitor_name_ou_id_project_agent
						UNIQUE(name, ou_id, project_name, agent_name),
					DROP CONSTRAINT IF EXISTS uq_monitor_name_org_project_agent`,
				`CREATE INDEX IF NOT EXISTS idx_monitor_ou_id ON monitors(ou_id)`,
				`DROP INDEX IF EXISTS idx_monitor_org`,

				// -- agent_configs --
				`ALTER TABLE agent_configs
					ADD CONSTRAINT uq_agent_config_agent_env_ou_id
						UNIQUE(ou_id, project_name, agent_name, environment_name),
					DROP CONSTRAINT IF EXISTS uq_agent_config_agent_env`,
				`CREATE INDEX IF NOT EXISTS idx_agent_configs_agent_ou_id
					ON agent_configs(ou_id, project_name, agent_name)`,
				`DROP INDEX IF EXISTS idx_agent_configs_agent`,

				// -- custom_evaluators (partial unique excludes soft-deleted rows;
				//    partial uniqueness is only expressible as an index) --
				`CREATE UNIQUE INDEX IF NOT EXISTS uq_custom_evaluator_ou_id_identifier
					ON custom_evaluators(ou_id, identifier)
					WHERE deleted_at IS NULL`,
				`DROP INDEX IF EXISTS uq_custom_evaluator_org_identifier`,
				`CREATE INDEX IF NOT EXISTS idx_custom_evaluator_ou_id ON custom_evaluators(ou_id)`,
				`DROP INDEX IF EXISTS idx_custom_evaluator_org`,

				// -- org_publisher_credentials (one credentials row per org) --
				`ALTER TABLE org_publisher_credentials
					ADD CONSTRAINT uq_org_publisher_creds_ou_id UNIQUE(ou_id),
					DROP CONSTRAINT IF EXISTS uq_org_publisher_creds_org`,
				`CREATE INDEX IF NOT EXISTS idx_org_publisher_creds_ou_id ON org_publisher_credentials(ou_id)`,
				`DROP INDEX IF EXISTS idx_org_publisher_creds_org`,

				// -- agent_kinds (plain unique constraint since 020) --
				`ALTER TABLE agent_kinds
					ADD CONSTRAINT uq_agent_kinds_ou_id_name UNIQUE(ou_id, name),
					DROP CONSTRAINT IF EXISTS uq_agent_kinds_org_name`,
				`CREATE INDEX IF NOT EXISTS idx_agent_kinds_ou_id ON agent_kinds(ou_id)`,
				`DROP INDEX IF EXISTS idx_agent_kinds_org`,

				// -- agent_thunder_clients --
				`ALTER TABLE agent_thunder_clients
					ADD CONSTRAINT uq_agent_thunder_clients_ou_id
						UNIQUE(ou_id, project_name, agent_name, environment_name),
					DROP CONSTRAINT IF EXISTS uq_agent_thunder_clients`,
				`CREATE INDEX IF NOT EXISTS idx_agent_thunder_clients_agent_ou_id
					ON agent_thunder_clients(ou_id, project_name, agent_name)`,
				`DROP INDEX IF EXISTS idx_agent_thunder_clients_agent`,
			)
		})
	},
}
