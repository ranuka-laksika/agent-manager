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

import { useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import {
  createAgentMCPConfig,
  deleteAgentMCPConfig,
  getAgentMCPConfig,
  listAgentMCPConfigs,
  updateAgentMCPConfig,
} from "../apis/agent-mcp-configs";
import type {
  AgentMCPConfigListResponse,
  AgentMCPConfigPathParams,
  AgentMCPConfigResponse,
  CreateAgentMCPConfigPathParams,
  CreateAgentMCPConfigRequest,
  DeleteAgentMCPConfigPathParams,
  ListAgentMCPConfigsPathParams,
  ListAgentMCPConfigsQuery,
  UpdateAgentMCPConfigPathParams,
  UpdateAgentMCPConfigRequest,
} from "@agent-management-platform/types";

const QUERY_KEY = "agent-mcp-configs";

export function useListAgentMCPConfigs(
  params: ListAgentMCPConfigsPathParams,
  query?: ListAgentMCPConfigsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentMCPConfigListResponse>({
    queryKey: [QUERY_KEY, "list", params, query],
    queryFn: () => listAgentMCPConfigs(params, query, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
  });
}

export function useGetAgentMCPConfig(params: AgentMCPConfigPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentMCPConfigResponse>({
    queryKey: [QUERY_KEY, params],
    queryFn: () => {
      if (!params.configId) throw new Error("configId is required");
      return getAgentMCPConfig(
        { ...params, configId: params.configId },
        getToken,
      );
    },
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.configId,
  });
}

export function useCreateAgentMCPConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentMCPConfigResponse,
    unknown,
    {
      params: CreateAgentMCPConfigPathParams;
      body: CreateAgentMCPConfigRequest;
    }
  >({
    action: { verb: "create", target: "agent MCP config" },
    mutationFn: ({ params, body }) =>
      createAgentMCPConfig(params, body, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
      queryClient.invalidateQueries({
        queryKey: ["agent-mcp-proxies", toAgentPathParams(variables.params)],
      });
    },
  });
}

export function useUpdateAgentMCPConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentMCPConfigResponse,
    unknown,
    {
      params: UpdateAgentMCPConfigPathParams;
      body: UpdateAgentMCPConfigRequest;
    }
  >({
    action: { verb: "update", target: "agent MCP config" },
    mutationFn: ({ params, body }) =>
      updateAgentMCPConfig(params, body, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
      queryClient.invalidateQueries({
        queryKey: ["agent-mcp-proxies", toAgentPathParams(variables.params)],
      });
    },
  });
}

export function useDeleteAgentMCPConfig() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteAgentMCPConfigPathParams>({
    action: { verb: "delete", target: "agent MCP config" },
    mutationFn: (params) => deleteAgentMCPConfig(params, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
      queryClient.invalidateQueries({
        queryKey: ["agent-mcp-proxies", toAgentPathParams(variables)],
      });
    },
  });
}

function toAgentPathParams(params: {
  orgName?: string;
  projName?: string;
  agentName?: string;
}) {
  return {
    orgName: params.orgName,
    projName: params.projName,
    agentName: params.agentName,
  };
}
