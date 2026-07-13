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

// migration033 creates mcp_proxy_scopes: one row per scope on an MCP proxy.
// A scope is an action on the resource server the proxy is; its token string is
// "<proxy-handle>:<action>". Scopes are per-proxy — they span all the proxy's
// endpoints (032) and environments. No org column — org scoping derives through
// the proxy FK (DB keys use ou_id/UUIDs, never the organization handle).
var migration033 = migration{
	ID: 33,
	Migrate: func(db *gorm.DB) error {
		sql := `
			CREATE TABLE mcp_proxy_scopes (
				uuid           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
				mcp_proxy_uuid UUID        NOT NULL REFERENCES mcp_proxies(uuid) ON DELETE CASCADE,
				action         TEXT        NOT NULL,
				description    TEXT        NOT NULL DEFAULT '',
				tools          JSONB       NOT NULL DEFAULT '[]',
				created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

				CONSTRAINT uq_mcp_proxy_scopes_proxy_action UNIQUE (mcp_proxy_uuid, action)
			);
			CREATE INDEX IF NOT EXISTS idx_mcp_proxy_scopes_proxy ON mcp_proxy_scopes (mcp_proxy_uuid);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}
