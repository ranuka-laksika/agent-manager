/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useListThunderInstances } from "@agent-management-platform/api-client";

interface UseThunderInstanceForEnvParams {
  orgId: string;
  envId: string;
}

/**
 * The org's Thunder instance bound to one environment (issuer/token/JWKS
 * URLs) — shared by the Manage AgentID drawer's OAuth2 Endpoints section and
 * the MCP server's "Connect" panel, both of which look up the same instance
 * by environment name from the org's full Thunder instance list.
 */
export function useThunderInstanceForEnv({ orgId, envId }: UseThunderInstanceForEnvParams) {
  const { data, isLoading, isError, error } = useListThunderInstances({ orgName: orgId });
  const thunderInstance = data?.thunderInstances.find(
    (instance) => instance.envName === envId,
  );

  return { thunderInstance, isLoading, isError, error };
}
