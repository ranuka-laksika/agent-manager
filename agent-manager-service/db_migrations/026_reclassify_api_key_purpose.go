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

// migration026 reclassifies existing api_keys.purpose values after the purpose
// scheme was split from {permanent=1, test=2} into
// {user-managed=1, test=2, console-managed=3}.
//
// Buckets:
//   - test keys (purpose = 2) are left untouched.
//   - non-test keys backing an Agent artifact are the agent Credentials-tab keys
//     and become user-managed (1).
//   - every other non-test key (LLM provider/proxy, MCP proxy/mapping, and any
//     key whose artifact row is missing) is console-managed (3).
var migration026 = migration{
	ID: 26,
	Migrate: func(db *gorm.DB) error {
		reclassify := `
		UPDATE api_keys k
		SET purpose = 3
		WHERE k.purpose <> 2
			AND NOT EXISTS (
				SELECT 1 FROM artifacts a
				WHERE a.uuid = k.artifact_uuid AND a.kind = 'Agent'
			);

		UPDATE api_keys k
		SET purpose = 1
		WHERE k.purpose <> 2
			AND EXISTS (
				SELECT 1 FROM artifacts a
				WHERE a.uuid = k.artifact_uuid AND a.kind = 'Agent'
			);
		`
		return db.Exec(reclassify).Error
	},
}
