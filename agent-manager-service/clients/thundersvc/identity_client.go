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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// IdentityClient provides user, group, and role management operations via the Thunder API.
type IdentityClient interface {
	// Users
	ListUsers(ctx context.Context, offset, limit int) ([]ThunderUser, int, error)
	GetUser(ctx context.Context, userID string) (*ThunderUser, error)
	CreateUser(ctx context.Context, req CreateUserRequest) (*ThunderUser, error)
	UpdateUser(ctx context.Context, userID string, req UpdateUserRequest) (*ThunderUser, error)
	DeleteUser(ctx context.Context, userID string) error
	GetUserGroups(ctx context.Context, userID string) ([]ThunderGroup, error)
	InviteUser(ctx context.Context, email string) (string, error)

	// Groups
	ListGroups(ctx context.Context, offset, limit int) ([]ThunderGroup, int, error)
	GetGroup(ctx context.Context, groupID string) (*ThunderGroup, error)
	CreateGroup(ctx context.Context, req CreateGroupRequest) (*ThunderGroup, error)
	UpdateGroup(ctx context.Context, groupID string, req UpdateGroupRequest) (*ThunderGroup, error)
	DeleteGroup(ctx context.Context, groupID string) error
	AddGroupMembers(ctx context.Context, groupID string, userIDs []string) error
	RemoveGroupMembers(ctx context.Context, groupID string, userIDs []string) error
	GetGroupMembers(ctx context.Context, groupID string, offset, limit int) ([]ThunderUser, int, error)

	// Roles
	ListRoles(ctx context.Context, offset, limit int) ([]ThunderRole, int, error)
	GetRole(ctx context.Context, roleID string) (*ThunderRole, error)
	CreateRole(ctx context.Context, req CreateRoleRequest) (*ThunderRole, error)
	UpdateRole(ctx context.Context, roleID string, req UpdateRoleRequest) (*ThunderRole, error)
	DeleteRole(ctx context.Context, roleID string) error
	GetRoleAssignments(ctx context.Context, roleID string) (*RoleAssignments, error)
	AddRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error
	RemoveRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error
	AddRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error
	RemoveRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error

	// Permissions catalog
	ListAMPPermissions(ctx context.Context) ([]ThunderPermission, string, error)

	// Organization units
	GetRootOUID(ctx context.Context) (string, error)
}

// NewIdentityClient creates a Thunder client for identity management operations.
// It shares the same transport and token-caching as ThunderClient.
func NewIdentityClient(baseURL, clientID, clientSecret string) IdentityClient {
	return &thunderClient{
		baseURL:      baseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: httpClientTimeout},
	}
}

// --- Users ---

func (c *thunderClient) ListUsers(ctx context.Context, offset, limit int) ([]ThunderUser, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/users?offset=%d&limit=%d", c.baseURL, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder list users: %w", err)
	}

	var wrapped thunderUserList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("thunder list users decode: %w", err)
	}
	return wrapped.Users, wrapped.TotalResults, nil
}

func (c *thunderClient) GetUser(ctx context.Context, userID string) (*ThunderUser, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/users/"+userID, token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get user: %w", err)
	}
	var user ThunderUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("thunder get user decode: %w", err)
	}
	return &user, nil
}

func (c *thunderClient) CreateUser(ctx context.Context, req CreateUserRequest) (*ThunderUser, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/users", token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder create user: %w", err)
	}
	var user ThunderUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("thunder create user decode: %w", err)
	}
	return &user, nil
}

func (c *thunderClient) UpdateUser(ctx context.Context, userID string, req UpdateUserRequest) (*ThunderUser, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/users/"+userID, token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder update user: %w", err)
	}
	var user ThunderUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("thunder update user decode: %w", err)
	}
	return &user, nil
}

func (c *thunderClient) DeleteUser(ctx context.Context, userID string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodDelete, c.baseURL+"/users/"+userID, token, nil)
	if err != nil {
		return fmt.Errorf("thunder delete user: %w", err)
	}
	return nil
}

func (c *thunderClient) GetUserGroups(ctx context.Context, userID string) ([]ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/users/"+userID+"/groups", token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get user groups: %w", err)
	}
	var groups []ThunderGroup
	if err := json.Unmarshal(body, &groups); err == nil {
		return groups, nil
	}
	var wrapped thunderGroupList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("thunder get user groups decode: %w", err)
	}
	return wrapped.Groups, nil
}

// --- Groups ---

func (c *thunderClient) ListGroups(ctx context.Context, offset, limit int) ([]ThunderGroup, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/groups?offset=%d&limit=%d", c.baseURL, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder list groups: %w", err)
	}
	var wrapped thunderGroupList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("thunder list groups decode: %w", err)
	}
	return wrapped.Groups, wrapped.TotalResults, nil
}

func (c *thunderClient) GetGroup(ctx context.Context, groupID string) (*ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/groups/"+groupID, token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get group: %w", err)
	}
	var group ThunderGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("thunder get group decode: %w", err)
	}
	return &group, nil
}

func (c *thunderClient) CreateGroup(ctx context.Context, req CreateGroupRequest) (*ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups", token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder create group: %w", err)
	}
	var group ThunderGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("thunder create group decode: %w", err)
	}
	return &group, nil
}

func (c *thunderClient) UpdateGroup(ctx context.Context, groupID string, req UpdateGroupRequest) (*ThunderGroup, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/groups/"+groupID, token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder update group: %w", err)
	}
	var group ThunderGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, fmt.Errorf("thunder update group decode: %w", err)
	}
	return &group, nil
}

func (c *thunderClient) DeleteGroup(ctx context.Context, groupID string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodDelete, c.baseURL+"/groups/"+groupID, token, nil)
	if err != nil {
		return fmt.Errorf("thunder delete group: %w", err)
	}
	return nil
}

func (c *thunderClient) AddGroupMembers(ctx context.Context, groupID string, userIDs []string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	members := make([]GroupMember, len(userIDs))
	for i, id := range userIDs {
		members[i] = GroupMember{ID: id, Type: "user"}
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups/"+groupID+"/members/add", token, GroupMembersRequest{Members: members})
	if err != nil {
		return fmt.Errorf("thunder add group members: %w", err)
	}
	return nil
}

func (c *thunderClient) RemoveGroupMembers(ctx context.Context, groupID string, userIDs []string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	members := make([]GroupMember, len(userIDs))
	for i, id := range userIDs {
		members[i] = GroupMember{ID: id, Type: "user"}
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/groups/"+groupID+"/members/remove", token, GroupMembersRequest{Members: members})
	if err != nil {
		return fmt.Errorf("thunder remove group members: %w", err)
	}
	return nil
}

func (c *thunderClient) GetGroupMembers(ctx context.Context, groupID string, offset, limit int) ([]ThunderUser, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/groups/%s/members?offset=%d&limit=%d", c.baseURL, groupID, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder get group members: %w", err)
	}
	var users []ThunderUser
	if err := json.Unmarshal(body, &users); err == nil {
		return users, len(users), nil
	}
	var wrapped thunderUserList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("thunder get group members decode: %w", err)
	}
	return wrapped.Users, wrapped.TotalResults, nil
}

// --- Roles ---

func (c *thunderClient) ListRoles(ctx context.Context, offset, limit int) ([]ThunderRole, int, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, 0, err
	}
	url := fmt.Sprintf("%s/roles?offset=%d&limit=%d", c.baseURL, offset, limit)
	body, err := c.doRequest(ctx, http.MethodGet, url, token, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("thunder list roles: %w", err)
	}
	var wrapped thunderRoleList
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("thunder list roles decode: %w", err)
	}
	return wrapped.Roles, wrapped.TotalResults, nil
}

func (c *thunderClient) GetRole(ctx context.Context, roleID string) (*ThunderRole, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/roles/"+roleID, token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get role: %w", err)
	}
	var role ThunderRole
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, fmt.Errorf("thunder get role decode: %w", err)
	}
	return &role, nil
}

func (c *thunderClient) CreateRole(ctx context.Context, req CreateRoleRequest) (*ThunderRole, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles", token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder create role: %w", err)
	}
	var role ThunderRole
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, fmt.Errorf("thunder create role decode: %w", err)
	}
	return &role, nil
}

func (c *thunderClient) UpdateRole(ctx context.Context, roleID string, req UpdateRoleRequest) (*ThunderRole, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodPut, c.baseURL+"/roles/"+roleID, token, req)
	if err != nil {
		return nil, fmt.Errorf("thunder update role: %w", err)
	}
	var role ThunderRole
	if err := json.Unmarshal(body, &role); err != nil {
		return nil, fmt.Errorf("thunder update role decode: %w", err)
	}
	return &role, nil
}

func (c *thunderClient) DeleteRole(ctx context.Context, roleID string) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodDelete, c.baseURL+"/roles/"+roleID, token, nil)
	if err != nil {
		return fmt.Errorf("thunder delete role: %w", err)
	}
	return nil
}

func (c *thunderClient) GetRoleAssignments(ctx context.Context, roleID string) (*RoleAssignments, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/roles/"+roleID+"/assignments", token, nil)
	if err != nil {
		return nil, fmt.Errorf("thunder get role assignments: %w", err)
	}
	var assignments RoleAssignments
	if err := json.Unmarshal(body, &assignments); err != nil {
		return nil, fmt.Errorf("thunder get role assignments decode: %w", err)
	}
	return &assignments, nil
}

func (c *thunderClient) AddRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles/"+roleID+"/assignments/add", token, req)
	if err != nil {
		return fmt.Errorf("thunder add role permissions: %w", err)
	}
	return nil
}

func (c *thunderClient) RemoveRolePermissions(ctx context.Context, roleID string, req RolePermissionRequest) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles/"+roleID+"/assignments/remove", token, req)
	if err != nil {
		return fmt.Errorf("thunder remove role permissions: %w", err)
	}
	return nil
}

func (c *thunderClient) AddRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles/"+roleID+"/assignments/add", token, req)
	if err != nil {
		return fmt.Errorf("thunder add role assignees: %w", err)
	}
	return nil
}

func (c *thunderClient) RemoveRoleAssignees(ctx context.Context, roleID string, req RoleAssignmentsRequest) error {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/roles/"+roleID+"/assignments/remove", token, req)
	if err != nil {
		return fmt.Errorf("thunder remove role assignees: %w", err)
	}
	return nil
}

// --- Permissions catalog ---

// ListAMPPermissions returns all permissions registered under the "amp" resource server.
// It returns the permissions as strings (e.g. "amp:agents:create") and the resource server ID.
func (c *thunderClient) ListAMPPermissions(ctx context.Context) ([]ThunderPermission, string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return nil, "", err
	}

	// Fetch resource servers and find the "amp" one
	rsBody, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/resource-servers?offset=0&limit=100", token, nil)
	if err != nil {
		return nil, "", fmt.Errorf("thunder list resource servers: %w", err)
	}

	var rsList thunderResourceServerList
	if err := json.Unmarshal(rsBody, &rsList); err != nil {
		// Try direct array
		var rsArr []ThunderResourceServer
		if err2 := json.Unmarshal(rsBody, &rsArr); err2 != nil {
			return nil, "", fmt.Errorf("thunder list resource servers decode: %w", err)
		}
		rsList.ResourceServers = rsArr
	}

	var ampRSID string
	for _, rs := range rsList.ResourceServers {
		if rs.Identifier == "amp" {
			ampRSID = rs.ID
			break
		}
	}
	if ampRSID == "" {
		return nil, "", fmt.Errorf("amp resource server not found in Thunder (has it been registered?)")
	}

	// Fetch resources for the amp resource server
	resBody, err := c.doRequest(ctx, http.MethodGet, c.baseURL+"/resource-servers/"+ampRSID+"/resources?offset=0&limit=500", token, nil)
	if err != nil {
		return nil, "", fmt.Errorf("thunder list amp resources: %w", err)
	}

	var resources []ThunderResource
	if err := json.Unmarshal(resBody, &resources); err != nil {
		var wrapped struct {
			Resources []ThunderResource `json:"resources"`
		}
		if err2 := json.Unmarshal(resBody, &wrapped); err2 != nil {
			return nil, "", fmt.Errorf("thunder list amp resources decode: %w", err)
		}
		resources = wrapped.Resources
	}

	// Derive permission strings: amp:{resource}:{action}
	var perms []ThunderPermission
	for _, res := range resources {
		for _, action := range res.Actions {
			perms = append(perms, ThunderPermission{
				Name:             "amp:" + res.Name + ":" + action.Name,
				ResourceServerID: ampRSID,
			})
		}
	}

	return perms, ampRSID, nil
}

// InviteUser executes Thunder's USER_ONBOARDING flow for the given email address and
// returns the invite link from the final step's additionalData.
func (c *thunderClient) InviteUser(ctx context.Context, email string) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", err
	}

	// Step 1: start the onboarding flow
	body1, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{"flowType": "USER_ONBOARDING", "verbose": true})
	if err != nil {
		return "", fmt.Errorf("thunder invite user start flow: %w", err)
	}
	var startResp struct {
		ExecutionID string `json:"executionId"`
	}
	if err := json.Unmarshal(body1, &startResp); err != nil {
		return "", fmt.Errorf("thunder invite user start flow decode: %w", err)
	}
	execID := startResp.ExecutionID

	// Step 2: select user type — Thunder requires this before accepting the email.
	_, err = c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{
			"executionId": execID,
			"inputs":      map[string]string{"userType": "engineer"},
			"verbose":     true,
			"action":      "action_usertype",
		})
	if err != nil {
		return "", fmt.Errorf("thunder invite user submit type: %w", err)
	}

	// Step 3: submit email and get invite link.
	body3, err := c.doRequest(ctx, http.MethodPost, c.baseURL+"/flow/execute", token,
		map[string]any{
			"executionId": execID,
			"inputs":      map[string]string{"email": email},
			"verbose":     true,
			"action":      "action_submit_email",
		})
	if err != nil {
		return "", fmt.Errorf("thunder invite user submit email: %w", err)
	}

	// Parse into a generic map so we can traverse whatever structure Thunder returns.
	var raw map[string]any
	if err := json.Unmarshal(body3, &raw); err != nil {
		return "", fmt.Errorf("thunder invite user submit email decode: %w", err)
	}

	link := extractInviteLink(raw)
	if link == "" {
		return "", fmt.Errorf("thunder invite user: inviteLink not found in response: %s", string(body3))
	}
	return link, nil
}

// extractInviteLink walks common Thunder flow response shapes looking for inviteLink.
func extractInviteLink(m map[string]any) string {
	// Candidate key names Thunder might use for the invite link.
	linkKeys := []string{"inviteLink", "invite_link", "link", "invitationLink"}

	// Check a map for any of the candidate keys.
	findLink := func(src map[string]any) string {
		for _, k := range linkKeys {
			if v, ok := src[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}

	// Top-level link field.
	if link := findLink(m); link != "" {
		return link
	}

	// Top-level additionalData / additionalInfo.
	for _, adKey := range []string{"additionalData", "additionalInfo"} {
		if ad, ok := m[adKey].(map[string]any); ok {
			if link := findLink(ad); link != "" {
				return link
			}
		}
	}

	// One level of wrapping (data / output / result / response).
	for _, wrapKey := range []string{"data", "output", "result", "response"} {
		if nested, ok := m[wrapKey].(map[string]any); ok {
			if link := findLink(nested); link != "" {
				return link
			}
			for _, adKey := range []string{"additionalData", "additionalInfo"} {
				if ad, ok := nested[adKey].(map[string]any); ok {
					if link := findLink(ad); link != "" {
						return link
					}
				}
			}
		}
	}
	return ""
}

// --- HTTP helper ---

// doRequest executes an authenticated HTTP request and returns the response body.
// For DELETE responses with no content it returns nil without error.
func (c *thunderClient) doRequest(ctx context.Context, method, url, token string, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil, &NotFoundError{Message: string(body)}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// NotFoundError is returned when Thunder responds with 404.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return "not found: " + e.Message
}

// IsNotFound returns true if the error is a Thunder 404.
func IsNotFound(err error) bool {
	var nfe *NotFoundError
	return errors.As(err, &nfe)
}

// GetRootOUID extracts the ouId claim from the system token JWT.
// The system app is registered in a specific Thunder OU; that OU is the correct
// target for all identity provisioning operations.
func (c *thunderClient) GetRootOUID(ctx context.Context) (string, error) {
	token, err := c.getSystemToken(ctx)
	if err != nil {
		return "", err
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("thunder system token is not a valid JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("thunder system token payload decode: %w", err)
	}
	var claims struct {
		OuID string `json:"ouId"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("thunder system token claims decode: %w", err)
	}
	if claims.OuID == "" {
		return "", fmt.Errorf("thunder system token missing ouId claim")
	}
	return claims.OuID, nil
}
