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

import { useGetAgentIdentity } from "@agent-management-platform/api-client";

interface AgentIdentityBindingParams {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Shared "is this environment's AgentID binding usable" read. Several
 * sibling sections per environment (the regenerate button, the identity
 * display, and the roles/groups list) each need to know whether
 * provisioning has completed — this centralizes that definition instead of
 * every consumer re-deriving `status === "completed"` from its own copy of
 * the binding.
 */
export function useAgentIdentityBinding({
  orgId, projectId, agentId, envId,
}: AgentIdentityBindingParams) {
  const { data: identityViews, isLoading } = useGetAgentIdentity(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { environment: envId },
  );
  const binding = identityViews?.[0];

  return {
    binding,
    provisioned: binding?.status === "completed",
    isLoading,
  };
}
