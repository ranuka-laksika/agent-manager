/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ListQuery, OrgPathParams, PaginationMeta } from './common';
import type { ThunderGroup, ThunderRole } from './identities';

// Env-Thunder group/role management for agent identities. AMS stores no
// group/role state of its own — every call is a passthrough to the
// environment's own Thunder instance. Roles carry catalog scopes as their
// permissions.

// --- Groups ---

export interface AgentIdentityGroupListResponse extends PaginationMeta {
  groups: ThunderGroup[];
}

export interface AgentIdentityGroupRequest {
  name: string;
  description?: string;
}

// Agent-identity group members are agents, not users.
export interface AgentIdentityMemberEntry {
  id: string;
  type: string; // "agent"
}

export interface AgentIdentityGroupMembersResponse extends PaginationMeta {
  members: AgentIdentityMemberEntry[];
}

export interface AgentIdentityMembersRequest {
  // Agent IDs (from the agents picker)
  agentIds: string[];
}

export interface AgentIdentityGroupRolesResponse {
  roles: ThunderRole[];
}

// --- Roles ---

export interface AgentIdentityRoleListResponse extends PaginationMeta {
  roles: ThunderRole[];
}

export interface AgentIdentityRoleRequest {
  name: string;
  description?: string;
  // Catalog scope names carried as the role's permissions
  scopes?: string[];
}

export type AgentIdentityAssigneeType = 'agent' | 'group';

export interface AgentIdentityAssignmentEntry {
  id: string;
  type: AgentIdentityAssigneeType;
}

export interface AgentIdentityAssignmentsRequest {
  assignments: AgentIdentityAssignmentEntry[];
}

export interface AgentIdentityRoleAssignmentsResponse {
  // Raw agent assignment entries; resolve display data via the agents picker.
  agents?: AgentIdentityAssignmentEntry[];
  groups?: ThunderGroup[];
}

// --- Agents picker ---

export interface AgentIdentityAgentResponse {
  agentName: string;
  projectName: string;
  // Thunder binding status (pending/in_progress/completed/failed)
  status: string;
  thunderAgentId?: string;
}

export interface AgentIdentityAgentListResponse {
  agents: AgentIdentityAgentResponse[];
}

// --- Scopes ---

export interface AgentIdentityScopeEntry {
  scope: string;
  description?: string;
  mcpProxyId: string;
  mcpProxyName?: string;
}

export interface AgentIdentityScopeListResponse {
  scopes: AgentIdentityScopeEntry[];
}

// --- Path params ---

export type AgentIdentityEnvPathParams = OrgPathParams & { envName: string };
export type AgentIdentityGroupPathParams = AgentIdentityEnvPathParams & { groupId: string };
export type AgentIdentityRolePathParams = AgentIdentityEnvPathParams & { roleId: string };

// --- Query params ---

export type AgentIdentityListQuery = ListQuery;
