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

// ThunderUser represents a user in Thunder.
type ThunderUser struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	OuID       string            `json:"ouId,omitempty"`
	Attributes map[string]any    `json:"attributes,omitempty"`
	Groups     []ThunderGroupRef `json:"groups,omitempty"`
	CreatedAt  string            `json:"createdAt,omitempty"`
	UpdatedAt  string            `json:"updatedAt,omitempty"`
}

// ThunderClaim is kept for compatibility in request bodies.
type ThunderClaim struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ThunderCred is kept for compatibility in request bodies.
type ThunderCred struct {
	Password string `json:"password"`
}

type ThunderGroupRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateUserRequest is the payload for POST /users.
type CreateUserRequest struct {
	OuID       string            `json:"ouId,omitempty"`
	Type       string            `json:"type"`
	Attributes map[string]string `json:"attributes"`
}

// UpdateUserRequest is the payload for PUT /users/{id}.
type UpdateUserRequest struct {
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ThunderGroup represents a user group in Thunder.
type ThunderGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// CreateGroupRequest is the payload for POST /groups.
type CreateGroupRequest struct {
	Name        string `json:"name"`
	OuID        string `json:"ouId"`
	Description string `json:"description,omitempty"`
}

// UpdateGroupRequest is the payload for PUT /groups/{id}.
type UpdateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GroupMember is a single member entry in a group members request.
type GroupMember struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "user" | "group" | "app" | "agent"
}

// GroupMembersRequest is the payload for /groups/{id}/members/add and remove.
type GroupMembersRequest struct {
	Members []GroupMember `json:"members"`
}

// ThunderRole represents a role in Thunder.
type ThunderRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// CreateRoleRequest is the payload for POST /roles.
type CreateRoleRequest struct {
	Name        string `json:"name"`
	OuID        string `json:"ouId"`
	Description string `json:"description,omitempty"`
}

// UpdateRoleRequest is the payload for PUT /roles/{id}.
type UpdateRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// RoleAssignments represents the current assignments on a role.
type RoleAssignments struct {
	Permissions []string       `json:"permissions,omitempty"`
	Users       []ThunderUser  `json:"users,omitempty"`
	Groups      []ThunderGroup `json:"groups,omitempty"`
}

// RolePermissionRequest is the payload for adding/removing permissions on a role.
type RolePermissionRequest struct {
	ResourceServerID string   `json:"resourceServerId"`
	Permissions      []string `json:"permissions"`
}

// AssignmentEntry is a single entry in a role assignments request.
type AssignmentEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "user" | "group" | "app" | "agent"
}

// RoleAssignmentsRequest is the payload for /roles/{id}/assignments/add and remove.
type RoleAssignmentsRequest struct {
	Assignments []AssignmentEntry `json:"assignments"`
}

// ThunderResourceServer represents a resource server registered in Thunder.
type ThunderResourceServer struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Identifier string            `json:"identifier"`
	Resources  []ThunderResource `json:"resources,omitempty"`
}

// ThunderResource represents a resource within a resource server.
type ThunderResource struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Actions []ThunderAction `json:"actions,omitempty"`
}

// ThunderAction represents an action on a resource.
type ThunderAction struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ThunderPermission is a derived permission string (e.g. "amp:agents:create").
type ThunderPermission struct {
	Name             string `json:"name"`
	ResourceServerID string `json:"resourceServerId"`
}

// thunderUserList is used to decode paginated user list responses.
type thunderUserList struct {
	Users        []ThunderUser `json:"users"`
	TotalResults int           `json:"totalResults"`
}

// thunderGroupMemberList decodes the GET /groups/{id}/members response.
type thunderGroupMemberList struct {
	TotalResults int           `json:"totalResults"`
	Members      []GroupMember `json:"members"`
}

// thunderGroupList is used to decode paginated group list responses.
type thunderGroupList struct {
	Groups       []ThunderGroup `json:"groups"`
	TotalResults int            `json:"totalResults"`
}

// thunderRoleList is used to decode paginated role list responses.
type thunderRoleList struct {
	Roles        []ThunderRole `json:"roles"`
	TotalResults int           `json:"totalResults"`
}

// thunderResourceServerList is used to decode paginated resource server list responses.
type thunderResourceServerList struct {
	ResourceServers []ThunderResourceServer `json:"resourceServers"`
	Total           int                     `json:"total"`
}
