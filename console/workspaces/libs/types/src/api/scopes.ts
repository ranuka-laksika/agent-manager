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

import type { OrgPathParams } from './common';

// Org-global, resource-agnostic scope catalog used as role permissions for
// agent identities.

export interface ScopeResponse {
  id: string;
  name: string;
  description?: string;
  createdAt?: string;
  updatedAt?: string;
  // Number of MCP proxy environment tool bindings referencing this scope
  bindingCount?: number;
}

export interface ScopeListResponse {
  scopes: ScopeResponse[];
}

export interface ScopeRequest {
  name: string;
  description?: string;
}

export interface ScopeUpdateRequest {
  description?: string;
}

// --- Path params ---

export type ListScopesPathParams = OrgPathParams;
export type CreateScopePathParams = OrgPathParams;
export type ScopePathParams = OrgPathParams & { scopeName: string | undefined };
export type UpdateScopePathParams = ScopePathParams;
export type DeleteScopePathParams = ScopePathParams;
