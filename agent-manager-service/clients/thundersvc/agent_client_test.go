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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestThunderServer builds an httptest server that always issues a valid
// system token, then delegates every other request to handle.
func newTestThunderServer(t *testing.T, handle http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-system-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/", handle)
	return httptest.NewServer(mux)
}

func TestCreateAgentIdentity_Success(t *testing.T) {
	var capturedBody map[string]any
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/agents", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "agent-uuid-1",
			"ouId":  "ou-1",
			"type":  "default",
			"name":  capturedBody["name"],
			"owner": capturedBody["owner"],
			"inboundAuthConfig": []map[string]any{
				{
					"type": "oauth2",
					"config": map[string]any{
						"clientId":     "generated-client-id",
						"clientSecret": "generated-client-secret",
						"grantTypes":   []string{"client_credentials"},
					},
				},
			},
		})
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	thunderAgentID, clientID, clientSecret, created, err := c.CreateAgentIdentity(context.Background(), "ou-1", "amp-agent-org-proj-myagent", "owner-subject-1")

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "agent-uuid-1", thunderAgentID)
	assert.Equal(t, "generated-client-id", clientID)
	assert.Equal(t, "generated-client-secret", clientSecret)

	assert.Equal(t, "ou-1", capturedBody["ouId"])
	assert.Equal(t, "default", capturedBody["type"])
	assert.Equal(t, "amp-agent-org-proj-myagent", capturedBody["name"])
	assert.Equal(t, "owner-subject-1", capturedBody["owner"])
	inbound, ok := capturedBody["inboundAuthConfig"].([]any)
	require.True(t, ok, "request must include inboundAuthConfig")
	require.Len(t, inbound, 1)
	entry := inbound[0].(map[string]any)
	assert.Equal(t, "oauth2", entry["type"])
	cfg := entry["config"].(map[string]any)
	grantTypes, _ := cfg["grantTypes"].([]any)
	require.Len(t, grantTypes, 1)
	assert.Equal(t, "client_credentials", grantTypes[0])
	assert.Equal(t, "client_secret_basic", cfg["tokenEndpointAuthMethod"])
}

func TestCreateAgentIdentity_DuplicateName_FallsBackToLookup(t *testing.T) {
	callCount := 0
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/agents":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    "AGT-1013",
				"message": "An agent with this name already exists",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/agents":
			callCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"totalResults": 1,
				"count":        1,
				"agents": []map[string]any{
					{"id": "existing-agent-uuid", "name": "amp-agent-org-proj-myagent", "clientId": "existing-client-id"},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	thunderAgentID, clientID, clientSecret, created, err := c.CreateAgentIdentity(context.Background(), "ou-1", "amp-agent-org-proj-myagent", "owner-subject-1")

	require.NoError(t, err)
	assert.False(t, created, "a duplicate-name conflict must not be reported as a fresh creation")
	assert.Equal(t, "existing-agent-uuid", thunderAgentID)
	assert.Equal(t, "existing-client-id", clientID)
	assert.Empty(t, clientSecret, "a looked-up existing agent's secret is never available")
	assert.GreaterOrEqual(t, callCount, 1, "must fall back to a lookup after the 409")
}

func TestCreateAgentIdentity_OtherErrorPropagates(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	_, _, _, _, err := c.CreateAgentIdentity(context.Background(), "ou-1", "some-agent", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestRegenerateAgentSecret_Success(t *testing.T) {
	var putBody map[string]any
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/agents/agent-uuid-1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "agent-uuid-1",
				"ouId":     "ou-1",
				"type":     "default",
				"name":     "amp-agent-org-proj-myagent",
				"clientId": "existing-client-id",
				"inboundAuthConfig": []map[string]any{
					{
						"type": "oauth2",
						"config": map[string]any{
							"clientId":                "existing-client-id",
							"grantTypes":              []string{"client_credentials"},
							"tokenEndpointAuthMethod": "client_secret_basic",
						},
					},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/agents/agent-uuid-1":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&putBody))
			_, hasID := putBody["id"]
			assert.False(t, hasID, "id must not be sent in the PUT body — it belongs in the URL")

			inbound := putBody["inboundAuthConfig"].([]any)
			cfg := inbound[0].(map[string]any)["config"].(map[string]any)
			newSecret := cfg["clientSecret"]
			require.NotEmpty(t, newSecret)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "agent-uuid-1",
				"name": "amp-agent-org-proj-myagent",
				"inboundAuthConfig": []map[string]any{
					{
						"type": "oauth2",
						"config": map[string]any{
							"clientId":     "existing-client-id",
							"clientSecret": newSecret,
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	newSecret, err := c.RegenerateAgentSecret(context.Background(), "agent-uuid-1")

	require.NoError(t, err)
	assert.NotEmpty(t, newSecret)
	assert.Len(t, newSecret, 64, "must use the same 384-bit random secret generator as the /applications path")
}

func TestRegenerateAgentSecret_NotFound(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	_, err := c.RegenerateAgentSecret(context.Background(), "missing-agent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestDeleteAgentIdentity_Success(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		require.True(t, strings.HasPrefix(r.URL.Path, "/agents/"))
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	found, err := c.DeleteAgentIdentity(context.Background(), "agent-uuid-1")
	require.NoError(t, err)
	assert.True(t, found)
}

func TestDeleteAgentIdentity_NotFoundIsNotAnError(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := NewThunderClient(srv.URL, "sys-client", "sys-secret")
	found, err := c.DeleteAgentIdentity(context.Background(), "missing-agent-id")
	require.NoError(t, err)
	assert.False(t, found)
}
