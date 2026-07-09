/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ListQuery, OrgPathParams } from "./common";
import type {
  SecurityConfig,
  UpstreamAuth,
  UpstreamConfig,
} from "./llm-providers";

export interface MCPProxyCapabilities {
  tools?: Record<string, unknown>[];
  resources?: Record<string, unknown>[];
  prompts?: Record<string, unknown>[];
}

export interface MCPProxyPolicy {
  name: string;
  version: string;
  displayName?: string;
  executionCondition?: string;
  params?: Record<string, unknown>;
}

export interface MCPToolScopeBinding {
  /** Name of the MCP tool the scopes gate */
  tool: string;
  /** Catalog scope names required to call the tool */
  scopes: string[];
}

export interface MCPPolicyAvailableItem {
  name: string;
  version: string;
}

export interface MCPPolicyAvailabilityResponse {
  count: number;
  list: MCPPolicyAvailableItem[];
}

/**
 * MCPEndpointConfig is the deployable configuration of a single MCP proxy endpoint:
 * upstream (URL + auth), policies, capabilities, security and tool-scope bindings. It is
 * the flat config carried on each MCPProxyEndpoint. Environment binding and per-environment
 * deployment status live on MCPEndpointEnvironment, not here.
 */
export interface MCPEndpointConfig {
  upstream?: UpstreamConfig;
  policies?: MCPProxyPolicy[];
  capabilities?: MCPProxyCapabilities;
  security?: SecurityConfig;
  toolScopeBindings?: MCPToolScopeBinding[];
}

/**
 * MCPEndpointEnvironment is one endpoint→environment binding. deploymentStatus is
 * response-only: it reports whether this environment's gateway artifact is currently
 * deployed ("Deployed") or not ("Undeployed"). Computed on read; never sent.
 */
export interface MCPEndpointEnvironment {
  environmentUuid: string;
  deploymentStatus?: "Deployed" | "Undeployed";
}

/**
 * MCPProxyEndpoint is one deployable endpoint of an MCP proxy. Its id is unique within the
 * parent proxy. The endpoint's flat config (upstream/policies/capabilities/security/
 * toolScopeBindings) applies to every environment it is bound to via environments; within a
 * proxy an environment maps to at most one endpoint.
 */
export interface MCPProxyEndpoint extends MCPEndpointConfig {
  id: string;
  name?: string;
  environments: MCPEndpointEnvironment[];
}

/**
 * MCPProxy is an org-level grouping. Name/version/context/vhost/mcpSpecVersion are shared
 * metadata; the deployable config lives on each endpoint in endpoints. The proxy itself
 * deploys nothing to any gateway.
 */
export interface MCPProxy {
  id: string;
  inCatalog?: boolean;
  name: string;
  version: string;
  description?: string;
  createdBy?: string;
  context?: string;
  vhost?: string;
  mcpSpecVersion?: string;
  endpoints: MCPProxyEndpoint[];
  createdAt?: string;
  updatedAt?: string;
}

export interface MCPProxyListItem {
  id?: string;
  name?: string;
  version?: string;
  description?: string;
  createdBy?: string;
  context?: string;
  status?: "pending" | "deployed" | "failed";
  mcpSpecVersion?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface MCPProxyPagination {
  count: number;
  limit: number;
  offset: number;
}

export interface MCPProxyListResponse {
  count: number;
  list: MCPProxyListItem[];
  pagination: MCPProxyPagination;
}

export interface MCPServerInfoFetchRequest {
  url?: string;
  proxyId?: string;
  auth?: UpstreamAuth;
}

export interface MCPServerInfoFetchResponse {
  serverInfo?: Record<string, unknown>;
  tools?: Record<string, unknown>[];
  resources?: Record<string, unknown>[];
  prompts?: Record<string, unknown>[];
}

export type CreateMCPProxyPathParams = OrgPathParams;
export type DeleteMCPProxyPathParams = OrgPathParams & { proxyId: string };
export type GetMCPProxyPathParams = OrgPathParams & { proxyId: string };
export type UpdateMCPProxyPathParams = OrgPathParams & { proxyId: string };
export type ListMCPProxiesPathParams = OrgPathParams;
export type ListAvailableMCPPoliciesPathParams = OrgPathParams;
export type ListMCPProxiesQuery = ListQuery;
export type FetchMCPProxyServerInfoPathParams = OrgPathParams;
