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

import { encodeRequired, httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  ScopeListResponse,
  ScopeResponse,
  ScopeRequest,
  ScopeUpdateRequest,
  ListScopesPathParams,
  CreateScopePathParams,
  UpdateScopePathParams,
  DeleteScopePathParams,
} from "@agent-management-platform/types";

const orgBase = (orgName: string) =>
  `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/scopes`;

/**
 * List the organization's scope catalog
 */
export async function listScopes(
  params: ListScopesPathParams,
  getToken?: () => Promise<string>,
): Promise<ScopeListResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(orgBase(orgName), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Create a new scope
 */
export async function createScope(
  params: CreateScopePathParams,
  body: ScopeRequest,
  getToken?: () => Promise<string>,
): Promise<ScopeResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(orgBase(orgName), body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Update a scope's description
 */
export async function updateScope(
  params: UpdateScopePathParams,
  body: ScopeUpdateRequest,
  getToken?: () => Promise<string>,
): Promise<ScopeResponse> {
  const { orgName = "default", scopeName } = params;
  const scope = encodeRequired(scopeName, "scopeName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(`${orgBase(orgName)}/${scope}`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Delete a scope. Fails with 409 while the scope is referenced by any MCP
 * proxy environment tool binding.
 */
export async function deleteScope(
  params: DeleteScopePathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", scopeName } = params;
  const scope = encodeRequired(scopeName, "scopeName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(`${orgBase(orgName)}/${scope}`, { token });
  if (!res.ok && res.status !== 204) throw await res.json();
}
