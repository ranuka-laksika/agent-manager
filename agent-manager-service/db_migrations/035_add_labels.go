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

// Add user-defined label support: a labels column on agent_kinds (filtered in
// SQL via @>) and an agent_labels sidecar table for agents, whose records live
// in the OpenChoreo control plane rather than this database.
var migration035 = migration{
	ID: 35,
	Migrate: func(db *gorm.DB) error {
		createAgentLabelsTable := `
		CREATE TABLE IF NOT EXISTS agent_labels (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id        VARCHAR(255) NOT NULL,
			project_name VARCHAR(63)  NOT NULL,
			agent_name   VARCHAR(63)  NOT NULL,
			labels       JSONB        NOT NULL DEFAULT '{}'::jsonb,
			created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

			CONSTRAINT uq_agent_labels_agent UNIQUE (ou_id, project_name, agent_name)
		)`

		createTrigger := `
		CREATE OR REPLACE FUNCTION update_agent_labels_updated_at()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS trg_agent_labels_updated_at ON agent_labels;
		CREATE TRIGGER trg_agent_labels_updated_at
			BEFORE UPDATE ON agent_labels
			FOR EACH ROW
			EXECUTE FUNCTION update_agent_labels_updated_at()`

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, `ALTER TABLE agent_kinds ADD COLUMN IF NOT EXISTS labels JSONB NOT NULL DEFAULT '{}'::jsonb`); err != nil {
				return err
			}
			if err := runSQL(tx, `CREATE INDEX IF NOT EXISTS idx_agent_kinds_labels ON agent_kinds USING GIN (labels jsonb_path_ops)`); err != nil {
				return err
			}
			if err := runSQL(tx, createAgentLabelsTable); err != nil {
				return err
			}
			if err := runSQL(tx, `CREATE INDEX IF NOT EXISTS idx_agent_labels_project ON agent_labels (ou_id, project_name)`); err != nil {
				return err
			}
			return runSQL(tx, createTrigger)
		})
	},
}
