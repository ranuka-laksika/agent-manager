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

// Adds ou_id (Thunder OU ID from the token) to every org-scoped table,
// i.e. any table with an organization_name or org_name column. The org-name
// columns are kept as display data only and get an empty-string default so
// inserts that no longer set them keep working (they are NOT NULL); ou_id is
// the org-scoping key from here on.
var migration029 = migration{
	ID: 29,
	Migrate: func(db *gorm.DB) error {
		orgColumnByTable := map[string]string{
			"gateways":                  "organization_name",
			"artifacts":                 "organization_name",
			"deployments":               "organization_name",
			"deployment_status":         "organization_name",
			"association_mappings":      "organization_name",
			"llm_provider_templates":    "organization_name",
			"agent_configurations":      "organization_name",
			"api_keys":                  "organization_name",
			"ai_applications":           "organization_name",
			"monitors":                  "org_name",
			"agent_configs":             "org_name",
			"custom_evaluators":         "org_name",
			"org_publisher_credentials": "org_name",
			"agent_kinds":               "org_name",
			"agent_thunder_clients":     "org_name",
		}
		for table, orgColumn := range orgColumnByTable {
			stmts := []string{
				`ALTER TABLE ` + table + ` ADD COLUMN IF NOT EXISTS ou_id VARCHAR(255) NOT NULL DEFAULT ''`,
				`ALTER TABLE ` + table + ` ALTER COLUMN ` + orgColumn + ` SET DEFAULT ''`,
			}
			for _, stmt := range stmts {
				if err := db.Exec(stmt).Error; err != nil {
					return err
				}
			}
		}
		return nil
	},
}
