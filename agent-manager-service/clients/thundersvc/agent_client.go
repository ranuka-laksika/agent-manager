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

package thundersvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// agentConflictCode is the error code Thunder returns (HTTP 409) when an /agents
// create request reuses a name that already exists.
const agentConflictCode = "AGT-1013"

// CreateAgentIdentity creates a client_credentials AgentID in Thunder via the
// /agents endpoint (not /applications — Thunder's agent-specific resource, verified
// against thunderid-0.45.0/backend/internal/agent and a live console capture).
// ouID is the Thunder organization unit to assign the agent to; owner is the
// Thunder subject recorded as the agent's owner (may be empty).
//
// Idempotent: if an agent with this name already exists, Thunder returns 409
// AGT-1013; in that case the existing agent is looked up by name and returned
// with created=false and no secret (Thunder only returns a secret at creation).
func (c *thunderClient) CreateAgentIdentity(ctx context.Context, ouID, name, owner string) (thunderAgentID, clientID, clientSecret string, created bool, err error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to get system token: %w", err)
	}

	payload := map[string]any{
		"ouId": ouID,
		"type": "default",
		"name": name,
		"inboundAuthConfig": []map[string]any{
			{
				"type": "oauth2",
				"config": map[string]any{
					"grantTypes":              []string{"client_credentials"},
					"tokenEndpointAuthMethod": "client_secret_basic",
				},
			},
		},
	}
	if owner != "" {
		payload["owner"] = owner
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/agents", bytes.NewReader(body))
	if err != nil {
		return "", "", "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", "", false, fmt.Errorf("thunder create agent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusConflict && bytes.Contains(respBody, []byte(agentConflictCode)) {
		existingID, existingClientID, findErr := c.findAgentByName(ctx, token, name)
		if findErr != nil {
			return "", "", "", false, findErr
		}
		if existingID == "" {
			return "", "", "", false, fmt.Errorf("thunder reported agent %q already exists but it could not be found by name", name)
		}
		return existingID, existingClientID, "", false, nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", "", false, fmt.Errorf("thunder create agent returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID          string `json:"id"`
		InboundAuth []struct {
			Config struct {
				ClientID     string `json:"clientId"`
				ClientSecret string `json:"clientSecret"`
			} `json:"config"`
		} `json:"inboundAuthConfig"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", "", "", false, fmt.Errorf("thunder create agent decode: %w", err)
	}
	if result.ID == "" {
		return "", "", "", false, fmt.Errorf("thunder create agent: id not found in response: %s", string(respBody))
	}
	if len(result.InboundAuth) == 0 || result.InboundAuth[0].Config.ClientID == "" {
		return "", "", "", false, fmt.Errorf("thunder create agent: clientId not found in response: %s", string(respBody))
	}

	slog.Info("Thunder agent identity created", "name", name, "thunderAgentID", result.ID)
	return result.ID, result.InboundAuth[0].Config.ClientID, result.InboundAuth[0].Config.ClientSecret, true, nil
}

// findAgentByName looks up an existing agent by exact name match, paginating
// through /agents. Returns empty strings if no agent with that name exists.
func (c *thunderClient) findAgentByName(ctx context.Context, token, name string) (thunderAgentID, clientID string, err error) {
	const (
		pageSize = 100
		maxPages = 100
	)
	for page := 0; page < maxPages; page++ {
		offset := page * pageSize
		reqURL := fmt.Sprintf("%s/agents?offset=%d&limit=%d", c.baseURL, offset, pageSize)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return "", "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", "", fmt.Errorf("thunder list agents: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return "", "", fmt.Errorf("thunder list agents read body: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("thunder list agents returned %d: %s", resp.StatusCode, string(body))
		}

		var page struct {
			Agents []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				ClientID string `json:"clientId"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return "", "", fmt.Errorf("thunder list agents decode: %w", err)
		}
		for _, a := range page.Agents {
			if a.Name == name {
				return a.ID, a.ClientID, nil
			}
		}
		if len(page.Agents) < pageSize {
			return "", "", nil
		}
	}
	return "", "", fmt.Errorf("thunder list agents exceeded %d pages looking for %s", maxPages, name)
}

// RegenerateAgentSecret generates a new client secret and applies it to the
// existing AgentID. Thunder's /agents API has no dedicated regenerate endpoint —
// PUT only auto-regenerates a secret when transitioning from a non-secret auth
// method, which does not apply to an already-secret-based client_credentials
// agent — so this explicitly supplies the new value, exactly like
// RegenerateClientSecret does for /applications.
func (c *thunderClient) RegenerateAgentSecret(ctx context.Context, thunderAgentID string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system token: %w", err)
	}

	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/agents/"+thunderAgentID, nil)
	if err != nil {
		return "", err
	}
	getReq.Header.Set("Authorization", "Bearer "+token)

	getResp, err := c.httpClient.Do(getReq)
	if err != nil {
		return "", fmt.Errorf("thunder get agent for secret regeneration: %w", err)
	}
	defer func() { _ = getResp.Body.Close() }()

	getBody, _ := io.ReadAll(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("thunder get agent returned %d: %s", getResp.StatusCode, string(getBody))
	}

	var agent map[string]any
	if err := json.Unmarshal(getBody, &agent); err != nil {
		return "", fmt.Errorf("thunder get agent decode: %w", err)
	}

	newSecret, err := generateRandomSecret()
	if err != nil {
		return "", fmt.Errorf("failed to generate client secret: %w", err)
	}
	if err := setInboundClientSecret(agent, newSecret); err != nil {
		return "", fmt.Errorf("failed to set client secret in agent payload: %w", err)
	}
	delete(agent, "id") // Thunder expects id in the URL, not the body

	putBody, _ := json.Marshal(agent)
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/agents/"+thunderAgentID, bytes.NewReader(putBody))
	if err != nil {
		return "", err
	}
	putReq.Header.Set("Authorization", "Bearer "+token)
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := c.httpClient.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("thunder put agent for secret regeneration: %w", err)
	}
	defer func() { _ = putResp.Body.Close() }()

	putRespBody, _ := io.ReadAll(putResp.Body)
	if putResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("thunder put agent returned %d: %s", putResp.StatusCode, string(putRespBody))
	}

	var result struct {
		InboundAuth []struct {
			Config struct {
				ClientSecret string `json:"clientSecret"`
			} `json:"config"`
		} `json:"inboundAuthConfig"`
	}
	if err := json.Unmarshal(putRespBody, &result); err != nil {
		return "", fmt.Errorf("thunder put agent response decode: %w", err)
	}
	if len(result.InboundAuth) == 0 || result.InboundAuth[0].Config.ClientSecret == "" {
		return "", fmt.Errorf("thunder put agent response missing clientSecret")
	}

	slog.Info("Thunder agent client secret regenerated", "thunderAgentID", thunderAgentID)
	return result.InboundAuth[0].Config.ClientSecret, nil
}

// DeleteAgentIdentity deletes the AgentID by its Thunder internal ID.
// Returns false (no error) if it did not exist — deletion is idempotent.
func (c *thunderClient) DeleteAgentIdentity(ctx context.Context, thunderAgentID string) (bool, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get system token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/agents/"+thunderAgentID, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("thunder delete agent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("thunder delete agent returned %d: %s", resp.StatusCode, string(body))
	}

	return true, nil
}

// GetDefaultOUID returns the default organization unit ID from Thunder.
// Exported wrapper around getDefaultOUID for callers outside this package
// (e.g. the AgentID provisioning service) that already hold a system token
// indirectly via this client but need the OU ID to create an agent.
func (c *thunderClient) GetDefaultOUID(ctx context.Context) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system token: %w", err)
	}
	return c.getDefaultOUID(ctx, token)
}
