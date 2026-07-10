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
import { listScopes, createScope, updateScope, deleteScope } from "../apis";
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
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

/**
 * Hook to list the organization's scope catalog
 */
export function useListScopes(params: ListScopesPathParams, options?: { enabled?: boolean }) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ScopeListResponse>({
    queryKey: ['scopes', params],
    queryFn: () => listScopes(params, getToken),
    enabled: options?.enabled ?? !!params.orgName,
  });
}

/**
 * Hook to create a new scope
 */
export function useCreateScope() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ScopeResponse, unknown, { params: CreateScopePathParams; body: ScopeRequest }
  >({
    action: { verb: 'create', target: 'scope' },
    mutationFn: ({ params, body }) => createScope(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['scopes', params] });
    },
  });
}

/**
 * Hook to update a scope's description
 */
export function useUpdateScope() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ScopeResponse, unknown, { params: UpdateScopePathParams; body: ScopeUpdateRequest }
  >({
    action: { verb: 'update', target: 'scope' },
    mutationFn: ({ params, body }) => updateScope(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['scopes', { orgName: params.orgName }] });
    },
  });
}

/**
 * Hook to delete a scope
 */
export function useDeleteScope() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteScopePathParams>({
    action: { verb: 'delete', target: 'scope' },
    mutationFn: (params) => deleteScope(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: ['scopes', { orgName: params.orgName }] });
    },
  });
}
