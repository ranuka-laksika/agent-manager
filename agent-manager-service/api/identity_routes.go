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

package api

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func registerIdentityRoutes(mux *http.ServeMux, ctrl controllers.IdentityController) {
	// Users
	middleware.HandleFuncWithValidationAndAnyAuthz(mux, "GET /orgs/{orgName}/identities/users", ctrl.ListUsers, rbac.OrgInviteMember, rbac.OrgRemoveMember)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/users/invite", rbac.OrgInviteMember, ctrl.InviteUser)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/users", rbac.OrgInviteMember, ctrl.CreateUser)
	middleware.HandleFuncWithValidationAndAnyAuthz(mux, "GET /orgs/{orgName}/identities/users/{userID}", ctrl.GetUser, rbac.OrgInviteMember, rbac.OrgRemoveMember)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/identities/users/{userID}", rbac.OrgInviteMember, ctrl.UpdateUser)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/identities/users/{userID}", rbac.OrgRemoveMember, ctrl.DeleteUser)
	middleware.HandleFuncWithValidationAndAnyAuthz(mux, "GET /orgs/{orgName}/identities/users/{userID}/groups", ctrl.GetUserGroups, rbac.OrgInviteMember, rbac.OrgRemoveMember)

	// Groups
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/groups", rbac.GroupRead, ctrl.ListGroups)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/groups", rbac.GroupCreate, ctrl.CreateGroup)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/groups/{groupID}", rbac.GroupRead, ctrl.GetGroup)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/identities/groups/{groupID}", rbac.GroupUpdate, ctrl.UpdateGroup)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/identities/groups/{groupID}", rbac.GroupDelete, ctrl.DeleteGroup)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/groups/{groupID}/members", rbac.GroupRead, ctrl.GetGroupMembers)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/groups/{groupID}/members/add", rbac.GroupUpdate, ctrl.AddGroupMembers)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/groups/{groupID}/members/remove", rbac.GroupUpdate, ctrl.RemoveGroupMembers)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/groups/{groupID}/roles", rbac.GroupRead, ctrl.GetGroupRoles)

	// Roles
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/roles", rbac.RoleRead, ctrl.ListRoles)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/roles", rbac.RoleCreate, ctrl.CreateRole)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/roles/{roleID}", rbac.RoleRead, ctrl.GetRole)
	middleware.HandleFuncWithValidationAndAuthz(mux, "PUT /orgs/{orgName}/identities/roles/{roleID}", rbac.RoleUpdate, ctrl.UpdateRole)
	middleware.HandleFuncWithValidationAndAuthz(mux, "DELETE /orgs/{orgName}/identities/roles/{roleID}", rbac.RoleDelete, ctrl.DeleteRole)
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/roles/{roleID}/assignments", rbac.RoleRead, ctrl.GetRoleAssignments)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/roles/{roleID}/permissions/add", rbac.RoleUpdate, ctrl.AddRolePermissions)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/roles/{roleID}/permissions/remove", rbac.RoleUpdate, ctrl.RemoveRolePermissions)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/roles/{roleID}/assignees/add", rbac.RoleUpdate, ctrl.AddRoleAssignees)
	middleware.HandleFuncWithValidationAndAuthz(mux, "POST /orgs/{orgName}/identities/roles/{roleID}/assignees/remove", rbac.RoleUpdate, ctrl.RemoveRoleAssignees)

	// Permissions catalog
	middleware.HandleFuncWithValidationAndAuthz(mux, "GET /orgs/{orgName}/identities/permissions", rbac.RoleRead, ctrl.ListAMPPermissions)
}
