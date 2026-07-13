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
import {
  createAgent, deleteAgent, getAgent, listAgents, generateAgentToken, updateAgent,
  updateAgentBuildParameters, getAgentRoles, getAgentGroups, getAgentIdentity,
  provisionAgentIdentity, regenerateAgentIdentitySecret, revokeAgentIdentitySecret,
  getAgentCredentials, claimAgentIdentitySecret,
} from "../apis";
import { SLOW_POLL_INTERVAL } from "../utils";
import type {
  AgentListResponse,
  AgentResponse,
  CreateAgentPathParams,
  CreateAgentRequest,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentsPathParams,
  ListAgentsQuery,
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
  GetAgentIdentityPathParams,
  GetAgentIdentityQuery,
  AgentIdentityEnvironmentView,
  ProvisionAgentIdentityPathParams,
  ProvisionAgentIdentityQuery,
  RegenerateAgentIdentitySecretPathParams,
  AgentIdentityActionRequest,
  AgentRegenerateSecretResponse,
  RevokeAgentIdentitySecretPathParams,
  RevokeAgentIdentitySecretQuery,
  AgentRevokeSecretResponse,
  GetAgentCredentialsPathParams,
  GetAgentCredentialsQuery,
  AgentCredentialsResponse,
  ClaimAgentIdentitySecretPathParams,
  ClaimAgentIdentitySecretQuery,
  AgentClaimSecretResponse,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

export function useListAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentListResponse>({
    queryKey: ['agents', params, query],
    queryFn: () => listAgents(params, query, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useGetAgent(params: GetAgentPathParams) {
    const { getToken } = useAuthHooks();
    return useApiQuery<AgentResponse>({
        queryKey: ['agent', params],
        queryFn: () => getAgent(params, getToken),
        enabled: !!params.orgName && !!params.projName && !!params.agentName,
    });
}

export function useCreateAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentResponse,
    unknown,
    { params: CreateAgentPathParams; body: CreateAgentRequest }
  >({
    action: { verb: 'create', target: 'agent' },
    mutationFn: ({ params, body }) => createAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

export function useUpdateAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentResponse,
    unknown,
    { params: UpdateAgentPathParams; body: UpdateAgentRequest }
  >({
    action: { verb: 'update', target: 'agent' },
    mutationFn: ({ params, body }) => updateAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agent'] });
    },
  });
}

export function useUpdateAgentBuildParameters() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentResponse,
    unknown,
    { params: UpdateAgentBuildParametersPathParams; body: UpdateAgentBuildParametersRequest }
  >({
    action: { verb: 'update', target: 'agent build parameters' },
    mutationFn: ({ params, body }) => updateAgentBuildParameters(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agent'] });
    },
  });
}

export function useDeleteAgent() {
    const { getToken } = useAuthHooks();
    const queryClient = useQueryClient();
    return useApiMutation<void, unknown, DeleteAgentPathParams>({
      action: { verb: 'delete', target: 'agent' },
        mutationFn: (params) => deleteAgent(params, getToken),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['agents'] });
        },
    });
}


export function useGenerateAgentToken(
  params: GenerateAgentTokenPathParams,
  body?: TokenRequest,
  query?: GenerateAgentTokenQuery,
  enabled: boolean = true
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<TokenResponse>({
    queryKey: ['agent-token', params.agentName, params.projName, params.orgName, body?.expires_in, query?.environment],
    queryFn: () => generateAgentToken(params, body, query, getToken),
    enabled: enabled
  });
}

// --- Agent identity: roles/groups (read-only) ---

export function useGetAgentRoles(
  params: GetAgentRolesPathParams,
  query: GetAgentRolesQuery,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRolesResponse>({
    queryKey: ['agent-roles', params, query],
    queryFn: () => getAgentRoles(params, query, getToken),
    enabled: (options?.enabled ?? true)
      && !!params.orgName && !!params.projName && !!params.agentName && !!query.environment,
  });
}

export function useGetAgentGroups(
  params: GetAgentGroupsPathParams,
  query: GetAgentGroupsQuery,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentGroupsResponse>({
    queryKey: ['agent-groups', params, query],
    queryFn: () => getAgentGroups(params, query, getToken),
    enabled: (options?.enabled ?? true)
      && !!params.orgName && !!params.projName && !!params.agentName && !!query.environment,
  });
}

// --- Agent identity: AgentID lifecycle (per environment) ---

export function useGetAgentIdentity(
  params: GetAgentIdentityPathParams,
  query?: GetAgentIdentityQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentIdentityEnvironmentView[]>({
    queryKey: ['agent-identity', params, query],
    queryFn: () => getAgentIdentity(params, query, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
    // Provisioning happens in the background (write-ahead PENDING, then a
    // best-effort attempt) — poll while any binding is still settling, and
    // stop automatically once every binding has completed or failed.
    refetchInterval: (q) => {
      const views = q.state.data;
      const stillProvisioning = views?.some(
        (v) => v.status === 'pending' || v.status === 'in_progress',
      );
      return stillProvisioning ? SLOW_POLL_INTERVAL : false;
    },
  });
}

export function useProvisionAgentIdentity() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentIdentityEnvironmentView,
    unknown,
    { params: ProvisionAgentIdentityPathParams; query: ProvisionAgentIdentityQuery }
  >({
    action: { verb: 'create', target: 'agent identity' },
    mutationFn: ({ params, query }) => provisionAgentIdentity(params, query, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}

export function useRegenerateAgentIdentitySecret() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentRegenerateSecretResponse,
    unknown,
    { params: RegenerateAgentIdentitySecretPathParams; body: AgentIdentityActionRequest }
  >({
    action: { verb: 'rotate', target: 'agent identity secret' },
    mutationFn: ({ params, body }) => regenerateAgentIdentitySecret(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}

export function useRevokeAgentIdentitySecret() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentRevokeSecretResponse,
    unknown,
    { params: RevokeAgentIdentitySecretPathParams; query: RevokeAgentIdentitySecretQuery }
  >({
    action: { verb: 'revoke', target: 'agent identity secret' },
    mutationFn: ({ params, query }) => revokeAgentIdentitySecret(params, query, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}

export function useGetAgentCredentials(
  params: GetAgentCredentialsPathParams,
  query: GetAgentCredentialsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentCredentialsResponse>({
    queryKey: ['agent-credentials', params, query],
    queryFn: () => getAgentCredentials(params, query, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!query.environment,
  });
}

export function useClaimAgentIdentitySecret() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentClaimSecretResponse,
    unknown,
    { params: ClaimAgentIdentitySecretPathParams; query: ClaimAgentIdentitySecretQuery }
  >({
    // No action/successMessage: the claimed secret itself is the success UI
    // (shown once, inline, with an explicit "won't be shown again" warning),
    // not a generic toast.
    mutationFn: ({ params, query }) => claimAgentIdentitySecret(params, query, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}
