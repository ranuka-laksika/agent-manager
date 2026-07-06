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
  UpstreamEndpoint,
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

export interface MCPPolicyAvailableItem {
  name: string;
  version: string;
}

export interface MCPPolicyAvailabilityResponse {
  count: number;
  list: MCPPolicyAvailableItem[];
}

/**
 * MCPEnvironmentConfig is one per-environment blueprint block on an org-level MCP proxy,
 * stored in MCPProxy.environments keyed by environment UUID. upstream holds the single
 * backend endpoint (URL + auth) for that environment.
 */
export interface MCPEnvironmentConfig {
  upstream?: UpstreamEndpoint;
  policies?: MCPProxyPolicy[];
  capabilities?: MCPProxyCapabilities;
  security?: SecurityConfig;
  /**
   * Response-only indicator of whether this environment's single gateway artifact is
   * currently deployed ("Deployed") or not ("Undeployed"). Computed on read; never sent.
   */
  deploymentStatus?: string;
}

/**
 * MCPProxy is an org-level blueprint. Name/version/context/vhost/mcpSpecVersion are shared
 * across environments; per-environment upstream, policies, capabilities and security live in
 * environments, keyed by environment UUID. The blueprint deploys nothing to any gateway.
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
  environments: Record<string, MCPEnvironmentConfig>;
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
