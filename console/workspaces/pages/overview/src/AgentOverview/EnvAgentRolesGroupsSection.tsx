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

import { Box, Chip, Divider, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import {
  useGetAgentGroups,
  useGetAgentRoles,
} from "@agent-management-platform/api-client";
import { useAgentIdentityBinding } from "./useAgentIdentityBinding";

interface EnvAgentRolesGroupsSectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

const ChipGroup: React.FC<{
  label: string;
  items: { id: string; name: string }[];
  emptyText: string;
}> = ({ label, items, emptyText }) => (
  <Box>
    <Typography variant="body2" color="text.secondary" mb={0.5}>
      {label}
    </Typography>
    {items.length === 0 ? (
      <Typography variant="body2" color="text.disabled">
        {emptyText}
      </Typography>
    ) : (
      <Stack direction="row" flexWrap="wrap" gap={1}>
        {items.map((item) => (
          <Chip key={item.id} label={item.name} size="small" />
        ))}
      </Stack>
    )}
  </Box>
);

interface UseAgentRolesAndGroupsParams {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
  enabled: boolean;
}

/**
 * Fetches the agent identity's effective Thunder roles/groups for one
 * environment. `enabled` should reflect whether the binding has completed
 * provisioning — the backend 404s (ErrAgentIdentityNotProvisioned) otherwise.
 * Shared by the standalone section below (internal agents) and
 * EnvAgentIdentitySection's combined layout (external agents).
 */
export function useAgentRolesAndGroups({
  orgId, projectId, agentId, envId, enabled,
}: UseAgentRolesAndGroupsParams) {
  const { data: rolesData, isLoading: isLoadingRoles } = useGetAgentRoles(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { environment: envId },
    { enabled },
  );
  const { data: groupsData, isLoading: isLoadingGroups } = useGetAgentGroups(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { environment: envId },
    { enabled },
  );

  return {
    roles: rolesData?.roles ?? [],
    groups: groupsData?.groups ?? [],
    isLoading: isLoadingRoles || isLoadingGroups,
  };
}

/**
 * Roles + Groups chip lists, stacked. Rendered on its own for internal
 * agents (EnvAgentRolesGroupsSection below), and as the right-hand column
 * of EnvAgentIdentitySection's combined row for external agents.
 */
export const RolesGroupsChips: React.FC<{
  roles: { id: string; name: string }[];
  groups: { id: string; name: string }[];
  isLoading: boolean;
}> = ({ roles, groups, isLoading }) => (
  isLoading ? (
    <Stack spacing={1.5}>
      <Skeleton variant="rounded" height={32} />
      <Skeleton variant="rounded" height={32} />
    </Stack>
  ) : (
    <Stack spacing={1.5}>
      <ChipGroup label="Roles" items={roles} emptyText="No roles assigned." />
      <ChipGroup label="Groups" items={groups} emptyText="Not a member of any groups." />
    </Stack>
  )
);

/**
 * Per-environment "Agent Identity" section for internal agents, rendered
 * inside an EnvironmentCard. External agents get the same roles/groups
 * content instead as part of EnvAgentIdentitySection's combined row, since
 * that section already owns an "Agent Identity" caption there.
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
      <Typography
        variant="caption"
        color="text.secondary"
        fontWeight={600}
        sx={{ textTransform: "uppercase", letterSpacing: "0.05em" }}
      >
        Agent Identity
      </Typography>

      <Box sx={{ mt: 1 }}>
        <RolesGroupsChips roles={roles} groups={groups} isLoading={isLoading} />
      </Box>
    </>
  );
};
