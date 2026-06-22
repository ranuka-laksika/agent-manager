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

package framework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// ampScopes is the full set of RBAC scopes the e2e suite requests for its
// client_credentials token. Thunder only includes scopes in a client_credentials
// token that are EXPLICITLY requested (it returns requested ∩ allowed), so when
// RBAC_ENABLED=true on Agent Manager, omitting these yields an unscoped token and
// every guarded route returns 403. This list mirrors the Permission constants in
// agent-manager-service/rbac/permissions.go; the IDP grants only the ones the
// client app is actually allowed, so requesting the superset is safe.
const ampScopes = "agent-kind:create agent-kind:delete agent-kind:read agent-kind:update " +
	"agent:api-key-manage agent:build agent:create agent:delete agent:deploy-non-production " +
	"agent:deploy-production agent:promote agent:read agent:rollback agent:suspend " +
	"agent:token-manage agent:update catalog:read data-plane:read " +
	"deployment-pipeline:create deployment-pipeline:delete deployment-pipeline:read deployment-pipeline:update " +
	"environment:create environment:delete environment:read environment:update " +
	"evaluator:create evaluator:delete evaluator:read evaluator:update " +
	"gateway:create gateway:delete gateway:read gateway:token-manage gateway:update " +
	"git-secret:create git-secret:delete git-secret:read " +
	"group:create group:delete group:read group:update " +
	"llm-provider-template:create llm-provider-template:delete llm-provider-template:read llm-provider-template:update " +
	"llm-provider:api-key-manage llm-provider:configure-guardrail llm-provider:connect llm-provider:create " +
	"llm-provider:delete llm-provider:deploy llm-provider:read llm-provider:update " +
	"llm-proxy:api-key-manage llm-proxy:create llm-proxy:delete llm-proxy:deploy llm-proxy:read llm-proxy:update " +
	"mcp-server:configure-guardrail mcp-server:connect mcp-server:create mcp-server:delete mcp-server:read mcp-server:update " +
	"monitor:create monitor:delete monitor:execute monitor:read monitor:score-publish monitor:score-read monitor:update " +
	"observability:guardrail-metric observability:infra-metric observability:org-dashboard observability:project-dashboard " +
	"org:assign-role org:invite-member org:manage-idp org:manage-service-account org:modify-settings org:remove-member org:view " +
	"project:create project:delete project:read project:update repository:read " +
	"role:create role:delete role:read role:update"

// FetchToken obtains an OAuth2 access token from the Thunder IDP using the
// client_credentials grant type. It retries on transient errors.
func FetchToken(cfg *Config) (string, error) {
	var lastErr error
	backoff := 2 * time.Second

	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			fmt.Printf("token fetch failed: %v, retrying in %v...\n", lastErr, backoff)
			time.Sleep(backoff)
			if backoff < 15*time.Second {
				backoff = backoff * 3 / 2
			}
		}

		token, err := fetchTokenOnce(cfg)
		if err == nil {
			return token, nil
		}
		lastErr = err
	}

	return "", lastErr
}

func fetchTokenOnce(cfg *Config) (string, error) {
	form := url.Values{
		"grant_type": {"client_credentials"},
		// Request the scopes explicitly — Thunder only embeds requested scopes in
		// a client_credentials token (returns requested ∩ allowed). Without this
		// the token is unscoped and RBAC-guarded routes return 403.
		"scope": {ampScopes},
	}

	req, err := http.NewRequest(http.MethodPost, cfg.IDPTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// amp-api-client uses client_secret_basic: credentials in Authorization header.
	req.SetBasicAuth(cfg.IDPClientID, cfg.IDPClientSecret)

	// kgateway routes by Host header; ensure it reaches Thunder
	parsedURL, err := url.Parse(cfg.IDPTokenURL)
	if err == nil && parsedURL.Hostname() != "" {
		req.Host = parsedURL.Host
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	if tok.AccessToken == "" {
		return "", fmt.Errorf("empty access_token in response: %s", string(body))
	}

	return tok.AccessToken, nil
}
