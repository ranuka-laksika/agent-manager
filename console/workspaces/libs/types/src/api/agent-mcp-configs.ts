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

import type {
  AgentModelConfigListResponse,
  AgentModelConfigPathParams,
  AgentModelConfigRequiredPathParams,
  AgentModelConfigResponse,
  CreateAgentModelConfigPathParams,
  CreateAgentModelConfigRequest,
  DeleteAgentModelConfigPathParams,
  EnvModelConfigRequest,
  ListAgentModelConfigsPathParams,
  ListAgentModelConfigsQuery,
  UpdateAgentModelConfigPathParams,
  UpdateAgentModelConfigRequest,
} from "./agent-model-configs";

export type AgentMCPConfigListResponse = AgentModelConfigListResponse;
export type AgentMCPConfigResponse = AgentModelConfigResponse;
export type EnvMCPProxyConfigRequest = Omit<
  EnvModelConfigRequest,
  "providerName" | "providerUuid"
> & {
  proxyName?: string;
  proxyId?: string;
  mcpProxyName?: string;
  mcpProxyId?: string;
  providerName?: string;
};
export type CreateAgentMCPConfigRequest = Omit<
  CreateAgentModelConfigRequest,
  "type" | "envMappings"
> & {
  type?: "mcp";
  envMappings: Record<string, EnvMCPProxyConfigRequest>;
};
export type UpdateAgentMCPConfigRequest = Omit<
  UpdateAgentModelConfigRequest,
  "envMappings"
> & {
  envMappings?: Record<string, EnvMCPProxyConfigRequest>;
};

export type ListAgentMCPConfigsPathParams = ListAgentModelConfigsPathParams;
export type CreateAgentMCPConfigPathParams = CreateAgentModelConfigPathParams;
export type AgentMCPConfigPathParams = AgentModelConfigPathParams;
export type AgentMCPConfigRequiredPathParams =
  AgentModelConfigRequiredPathParams;
export type GetAgentMCPConfigPathParams = AgentMCPConfigRequiredPathParams;
export type UpdateAgentMCPConfigPathParams = UpdateAgentModelConfigPathParams;
export type DeleteAgentMCPConfigPathParams = DeleteAgentModelConfigPathParams;
export type ListAgentMCPConfigsQuery = ListAgentModelConfigsQuery;
