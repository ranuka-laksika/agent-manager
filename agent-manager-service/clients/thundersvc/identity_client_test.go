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
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexOf returns the position of s in ss, or -1 if absent.
func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

func TestAddGroupMemberEntries_SendsAgentType(t *testing.T) {
	var body GroupMembersRequest
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/groups/g1/members/add", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	err := c.AddGroupMemberEntries(context.Background(), "g1", []GroupMember{{ID: "a1", Type: "agent"}})

	require.NoError(t, err)
	require.Len(t, body.Members, 1)
	assert.Equal(t, "a1", body.Members[0].ID)
	assert.Equal(t, "agent", body.Members[0].Type)
}

func TestRemoveGroupMemberEntries_SendsAgentType(t *testing.T) {
	var body GroupMembersRequest
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/groups/g1/members/remove", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	err := c.RemoveGroupMemberEntries(context.Background(), "g1", []GroupMember{{ID: "a1", Type: "agent"}})

	require.NoError(t, err)
	require.Len(t, body.Members, 1)
	assert.Equal(t, "a1", body.Members[0].ID)
	assert.Equal(t, "agent", body.Members[0].Type)
}

func TestListGroupMemberEntries_ReturnsTypedEntries(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/groups/g1/members", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalResults": 2,
			"members": []map[string]any{
				{"id": "a1", "type": "agent"},
				{"id": "u1", "type": "user"},
			},
		})
	})
	defer srv.Close()

	c := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	members, total, err := c.ListGroupMemberEntries(context.Background(), "g1", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, members, 2)
	assert.Equal(t, GroupMember{ID: "a1", Type: "agent"}, members[0])
	assert.Equal(t, GroupMember{ID: "u1", Type: "user"}, members[1])
}

func TestEnsureProxyResourceServer_CreatesRSWithHandleAndRootActions(t *testing.T) {
	rsCreated, actCreated := 0, 0
	var createRSBody map[string]string
	var createActionBodies []map[string]string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{}, "total": 0})
		case r.Method == http.MethodGet && r.URL.Path == "/organization-units/tree/default":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "ou-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			rsCreated++
			_ = json.NewDecoder(r.Body).Decode(&createRSBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "rs-1", "identifier": "gh-proxy"})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}, "totalResults": 0})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/actions":
			actCreated++
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			createActionBodies = append(createActionBodies, body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": fmt.Sprintf("act-%d", actCreated), "handle": body["handle"], "permission": "gh-proxy:" + body["handle"]})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", []string{"read", "write"})
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
	assert.Equal(t, 1, rsCreated)
	assert.Equal(t, 2, actCreated)
	assert.Equal(t, "gh-proxy", createRSBody["handle"], "RS handle must be the proxy handle — it prefixes derived permissions")
	assert.Equal(t, "gh-proxy", createRSBody["identifier"])
	assert.Equal(t, ":", createRSBody["delimiter"])
	assert.Equal(t, "MCP", createRSBody["type"])
	assert.Equal(t, "ou-1", createRSBody["ouId"])
	assert.Len(t, createActionBodies, 2)
}

func TestEnsureProxyResourceServer_IdempotentSkipsExistingActions(t *testing.T) {
	rsCreated, actCreated := 0, 0
	var createActionBodies []map[string]string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceServers": []any{map[string]string{"id": "rs-1", "identifier": "gh-proxy"}},
				"total":           1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers":
			rsCreated++
			t.Fatalf("no RS create expected when the resource server already exists")
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"actions":      []any{map[string]string{"id": "act-1", "handle": "read"}},
				"totalResults": 1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/resource-servers/rs-1/actions":
			actCreated++
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			createActionBodies = append(createActionBodies, body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "act-2", "handle": body["handle"]})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	rsID, err := client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", []string{"read", "write"})
	assert.NoError(t, err)
	assert.Equal(t, "rs-1", rsID)
	assert.Equal(t, 0, rsCreated)
	require.Len(t, createActionBodies, 1, "only the missing action must be created")
	assert.Equal(t, "write", createActionBodies[0]["handle"])
}

func TestEnsureProxyResourceServer_RejectsOverlongInputs(t *testing.T) {
	srv := newTestThunderServer(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("no Thunder calls expected for over-long input, got %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")

	_, err := client.EnsureProxyResourceServer(context.Background(), strings.Repeat("h", 101), "Too Long", []string{"read"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "100", "over-long handle error should state the 100-character Thunder limit")

	_, err = client.EnsureProxyResourceServer(context.Background(), "gh-proxy", "GitHub Proxy", []string{strings.Repeat("a", 101)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "100", "over-long action error should state the 100-character Thunder limit")
}

func TestDeleteProxyResourceServer_DeletesActionsThenRS(t *testing.T) {
	var calls []string
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceServers": []any{map[string]string{"id": "rs-1", "identifier": "gh-proxy"}}, "total": 1})
		case r.Method == http.MethodGet && r.URL.Path == "/resource-servers/rs-1/actions":
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{map[string]string{"id": "act-1", "handle": "read"}}, "totalResults": 1})
		case r.Method == http.MethodDelete && r.URL.Path == "/resource-servers/rs-1/actions/act-1":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/resource-servers/rs-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()
	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	assert.NoError(t, client.DeleteProxyResourceServer(context.Background(), "gh-proxy"))
	// action delete MUST precede RS delete (Thunder 400-blocks otherwise)
	assert.Less(t, indexOf(calls, "DELETE /resource-servers/rs-1/actions/act-1"), indexOf(calls, "DELETE /resource-servers/rs-1"))
}

// TestGetAgentRoleAssignments_ReturnsAgentEntriesAndResolvedGroups proves the
// agent-identity read path keeps agent assignees (as raw entries) and resolves
// group assignees, unlike the user-store GetRoleAssignments which drops agents.
func TestGetAgentRoleAssignments_ReturnsAgentEntriesAndResolvedGroups(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/roles/r1/assignments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"assignments": []map[string]string{
					{"id": "a1", "type": "agent"},
					{"id": "g1", "type": "group"},
					{"id": "a2", "type": "agent"},
					{"id": "u1", "type": "user"},
				},
			})
		case "/groups/g1":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "g1", "name": "readers"})
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	assignments, err := client.GetAgentRoleAssignments(context.Background(), "r1")

	require.NoError(t, err)
	assert.Equal(t, []AssignmentEntry{{ID: "a1", Type: "agent"}, {ID: "a2", Type: "agent"}}, assignments.Agents)
	require.Len(t, assignments.Groups, 1)
	assert.Equal(t, "readers", assignments.Groups[0].Name)
}

// TestGetAgentRoleAssignments_SkipsDeletedGroup proves a group assignee that no
// longer exists in Thunder is skipped rather than failing the whole listing.
func TestGetAgentRoleAssignments_SkipsDeletedGroup(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/roles/r1/assignments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"assignments": []map[string]string{
					{"id": "g-gone", "type": "group"},
					{"id": "a1", "type": "agent"},
				},
			})
		case "/groups/g-gone":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected call %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	assignments, err := client.GetAgentRoleAssignments(context.Background(), "r1")

	require.NoError(t, err)
	assert.Empty(t, assignments.Groups)
	assert.Equal(t, []AssignmentEntry{{ID: "a1", Type: "agent"}}, assignments.Agents)
}

// TestListRoles_OUFiltered_ExcludesNativeAdministrator proves the OU-scoped role
// listing hides Thunder's native Administrator role — it carries the built-in
// "system" scope and must never surface as an assignable agent role — and that
// the exclusion happens before client-side pagination so offset/limit/total stay
// consistent.
func TestListRoles_OUFiltered_ExcludesNativeAdministrator(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/roles", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalResults": 3,
			"roles": []map[string]any{
				{"id": "r-admin", "ouId": "ou-1", "name": NativeAdministratorRoleName},
				{"id": "r-readers", "ouId": "ou-1", "name": "readers"},
				{"id": "r-elsewhere", "ouId": "ou-2", "name": "writers"},
			},
		})
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	roles, total, err := client.ListRoles(context.Background(), "ou-1", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 1, total, "Administrator must not count toward the OU total")
	require.Len(t, roles, 1)
	assert.Equal(t, "readers", roles[0].Name)
}

// TestListRoles_Unfiltered_KeepsNativeAdministrator pins the ouID="" contract:
// rolesForAssignee and the scope-cleanup sweep walk every role in the instance
// and must still see the native Administrator role (e.g. to report an agent
// already mis-assigned to it).
func TestListRoles_Unfiltered_KeepsNativeAdministrator(t *testing.T) {
	srv := newTestThunderServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalResults": 2,
			"roles": []map[string]any{
				{"id": "r-admin", "ouId": "ou-1", "name": NativeAdministratorRoleName},
				{"id": "r-readers", "ouId": "ou-1", "name": "readers"},
			},
		})
	})
	defer srv.Close()

	client := NewIdentityClient(srv.URL, "sys-client", "sys-secret")
	roles, total, err := client.ListRoles(context.Background(), "", 0, 20)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, roles, 2)
}
