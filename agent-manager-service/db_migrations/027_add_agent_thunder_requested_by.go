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

// migration027 adds requested_by: the calling user's own subject (from AMS's
// existing incoming-request auth), captured for audit purposes only. Thunder's
// own "owner" field on an AgentID cannot carry this — each env-Thunder is a
// fully isolated instance with its own entity store, so a platform-Thunder
// user ID does not resolve there (confirmed live: env-Thunder rejects it with
// AGT-1039 "owner not found"). This column is AMS's own record of who asked,
// independent of what Thunder's API will accept.
var migration027 = migration{
	ID: 27,
	Migrate: func(db *gorm.DB) error {
		sql := `
			ALTER TABLE agent_thunder_clients
				ADD COLUMN IF NOT EXISTS requested_by TEXT NOT NULL DEFAULT '';
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}
