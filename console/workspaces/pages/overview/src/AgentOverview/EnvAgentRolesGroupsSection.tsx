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

import { Box, Divider, Skeleton } from "@wso2/oxygen-ui";
import { useAgentIdentityBinding } from "@agent-management-platform/api-client";
import {
  RolesGroupsChips,
  useAgentRolesAndGroups,
} from "@agent-management-platform/shared-component";
import { buildManageIdentityHref } from "./manageIdentityLink";
import { SectionHeader } from "./SectionHeader";

interface EnvAgentRolesGroupsSectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Per-environment "Agent Identity" roles/groups display, rendered inside an
 * EnvironmentCard for both internal and external agents — the client
 * ID/secret/regenerate flow lives on the Configure Agent page's "Manage
 * AgentID" drawer instead, linked to via the "View all" button here (styled
 * the same way as the Agent Performance / Recent Traces section headers).
 */
export const EnvAgentRolesGroupsSection: React.FC<EnvAgentRolesGroupsSectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  const { provisioned, isLoading: isLoadingIdentity } = useAgentIdentityBinding({
    orgId, projectId, agentId, envId,
  });

  const { roles, groups, isLoading } = useAgentRolesAndGroups({
    orgId, projectId, agentId, envId, enabled: provisioned,
  });

  if (isLoadingIdentity) {
    return <Skeleton variant="rounded" height={56} sx={{ mt: 2 }} />;
  }

  if (!provisioned) {
    return null;
  }

  return (
    <>
      <Divider sx={{ mt: 2, mb: 1 }} />
      <SectionHeader
        title="Agent Identity"
        viewAllHref={buildManageIdentityHref(orgId, projectId, agentId, envId)}
      />

      <Box sx={{ mt: 1 }}>
        <RolesGroupsChips roles={roles} groups={groups} isLoading={isLoading} />
      </Box>
    </>
  );
};
