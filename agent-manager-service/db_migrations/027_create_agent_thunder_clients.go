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

var migration027 = migration{
	ID: 27,
	Migrate: func(db *gorm.DB) error {
		sql := `
			CREATE TABLE agent_thunder_clients (
				id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
				org_name          TEXT        NOT NULL,
				project_name      TEXT        NOT NULL,
				agent_name        TEXT        NOT NULL,
				environment_name  TEXT        NOT NULL,
				provisioning_type    TEXT        NOT NULL CHECK (provisioning_type IN ('internal', 'external')),
				thunder_agent_id  TEXT        NOT NULL DEFAULT '',
				thunder_client_id TEXT        NOT NULL DEFAULT '',
				secret_ref_path   TEXT        NOT NULL DEFAULT '',
				status            TEXT        NOT NULL DEFAULT 'pending'
					CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
				attempt_count     INT         NOT NULL DEFAULT 0,
				last_error        TEXT        NOT NULL DEFAULT '',
				last_attempted_at TIMESTAMPTZ,
				next_retry_at     TIMESTAMPTZ,
				claimed_at        TIMESTAMPTZ,
				created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

				CONSTRAINT uq_agent_thunder_clients UNIQUE (org_name, project_name, agent_name, environment_name)
			);
			CREATE INDEX IF NOT EXISTS idx_agent_thunder_clients_agent
				ON agent_thunder_clients (org_name, project_name, agent_name);
			CREATE INDEX IF NOT EXISTS idx_agent_thunder_clients_retry_due
				ON agent_thunder_clients (status, next_retry_at);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}
