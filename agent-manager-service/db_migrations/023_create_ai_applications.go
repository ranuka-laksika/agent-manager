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

// Create ai_applications table to track per-agent-per-environment applications for
// consumer-level rate limiting.
var migration023 = migration{
	ID: 23,
	Migrate: func(db *gorm.DB) error {
		createAIApplicationsTable := `
		CREATE TABLE IF NOT EXISTS ai_applications (
			uuid              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			handle            VARCHAR(255) NOT NULL,
			name              VARCHAR(255) NOT NULL,
			agent_id          VARCHAR(255) NOT NULL,
			project_name      VARCHAR(255) NOT NULL,
			environment_name  VARCHAR(255) NOT NULL,
			organization_name VARCHAR(255) NOT NULL,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(organization_name, project_name, agent_id, environment_name)
		);

		CREATE INDEX IF NOT EXISTS idx_ai_applications_agent ON ai_applications(organization_name, project_name, agent_id);
		CREATE INDEX IF NOT EXISTS idx_ai_applications_org   ON ai_applications(organization_name);
		`
		return db.Exec(createAIApplicationsTable).Error
	},
}
