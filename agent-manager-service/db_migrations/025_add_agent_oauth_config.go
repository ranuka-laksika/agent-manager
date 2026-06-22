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

var migration025 = migration{
	ID: 25,
	Migrate: func(db *gorm.DB) error {
		addOAuthConfig := `
		ALTER TABLE agent_configs
			ADD COLUMN IF NOT EXISTS enable_oauth_security    BOOLEAN NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS oauth_issuers            JSONB   NOT NULL DEFAULT '[]',
			ADD COLUMN IF NOT EXISTS oauth_audiences          JSONB   NOT NULL DEFAULT '[]',
			ADD COLUMN IF NOT EXISTS oauth_header_name        TEXT    NOT NULL DEFAULT 'Authorization',
			ADD COLUMN IF NOT EXISTS oauth_auth_header_prefix TEXT    NOT NULL DEFAULT 'Bearer',
			ADD COLUMN IF NOT EXISTS oauth_forward_token      BOOLEAN NOT NULL DEFAULT true;
		`
		if err := db.Exec(addOAuthConfig).Error; err != nil {
			return err
		}

		// gateway_identity_providers mirrors the gateway-side jwt-auth keymanagers
		// (token issuers). The gateway ConfigMap remains the source of truth; this
		// table is written by the gateway bootstrap (seed) and the management script.
		createIdentityProviders := `
		CREATE TABLE IF NOT EXISTS gateway_identity_providers (
			uuid                 UUID         PRIMARY KEY,
			gateway_uuid         UUID         NOT NULL REFERENCES gateways(uuid) ON DELETE CASCADE,
			name                 TEXT         NOT NULL,
			issuer               TEXT         NOT NULL DEFAULT '',
			jwks_uri             TEXT         NOT NULL DEFAULT '',
			description          TEXT         NOT NULL DEFAULT '',
			type                 TEXT         NOT NULL DEFAULT 'custom',
			jwks_skip_tls_verify BOOLEAN      NOT NULL DEFAULT false,
			created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
			updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
			UNIQUE (gateway_uuid, name)
		);
		CREATE INDEX IF NOT EXISTS idx_gateway_identity_providers_gateway_uuid
			ON gateway_identity_providers (gateway_uuid);
		`
		return db.Exec(createIdentityProviders).Error
	},
}
