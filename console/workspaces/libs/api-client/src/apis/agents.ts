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

import { cloneDeep } from "lodash";
import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  AgentListResponse,
  AgentResponse,
  CreateAgentPathParams,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentsPathParams,
  ListAgentsQuery,
  CreateAgentRequest,
  UpdateAgentPathParams,
  UpdateAgentRequest,
  UpdateAgentBuildParametersPathParams,
  UpdateAgentBuildParametersRequest,
  GenerateAgentTokenPathParams,
  GenerateAgentTokenQuery,
  TokenRequest,
  TokenResponse,
  GetAgentRolesPathParams,
  GetAgentRolesQuery,
  AgentRolesResponse,
  GetAgentGroupsPathParams,
  GetAgentGroupsQuery,
  AgentGroupsResponse,
} from "@agent-management-platform/types";

export async function listAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentListResponse> {
  const { orgName = "default", projName = "default" } = params;

  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents`,
    { searchParams: search, token: token }
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createAgent(
  params: CreateAgentPathParams,
  body: CreateAgentRequest,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents`,
    cloneDeep(body),
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgent(
  params: GetAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}`;
  const res = await httpGET(url, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgent(
  params: DeleteAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}`;
  const res = await httpDELETE(url, { token });
  if (!res.ok) throw await res.json();
}

export async function updateAgent(
  params: UpdateAgentPathParams,
  body: UpdateAgentRequest,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}`;
  const res = await httpPUT(url, cloneDeep(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentBuildParameters(
  params: UpdateAgentBuildParametersPathParams,
  body: UpdateAgentBuildParametersRequest,
  getToken?: () => Promise<string>,
): Promise<AgentResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}/build-parameters`;
  const res = await httpPUT(url, cloneDeep(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function generateAgentToken(
  params: GenerateAgentTokenPathParams,
  body?: TokenRequest,
  query?: GenerateAgentTokenQuery,
  getToken?: () => Promise<string>,
): Promise<TokenResponse> {
  const { orgName = "default", projName = "default", agentName } = params;

  if (!agentName) {
    throw new Error("agentName is required");
  }

  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;

  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName)}/token`;
  
  const res = await httpPOST(url, body || {}, { searchParams: search, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Agent identity: roles/groups (read-only) ---

export async function getAgentRoles(
  params: GetAgentRolesPathParams,
  query: GetAgentRolesQuery,
  getToken?: () => Promise<string>,
): Promise<AgentRolesResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName ?? "")}/roles`;
  const res = await httpGET(url, { searchParams: { environment: query.environment }, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentGroups(
  params: GetAgentGroupsPathParams,
  query: GetAgentGroupsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentGroupsResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  const token = getToken ? await getToken() : undefined;
  const url =
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}` +
    `/projects/${encodeURIComponent(projName)}` +
    `/agents/${encodeURIComponent(agentName ?? "")}/groups`;
  const res = await httpGET(url, { searchParams: { environment: query.environment }, token });
  if (!res.ok) throw await res.json();
  return res.json();
}
