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

import React, { useMemo, useState } from "react";
import { Alert, Box, Chip, Form, Stack, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import { generatePath, useParams } from "react-router-dom";
import {
  useGetAgent,
  useGetAgentGroups,
  useGetAgentRoles,
  useGetProject,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  BackButton,
  EditFormSkeleton,
  EntityHeader,
} from "@agent-management-platform/shared-component";

type TabId = "roles" | "groups";

// Read-only: this page just shows the agent's effective roles/groups in this
// environment. Assignment happens from the Roles/Groups pages, same as how
// UserEditPage's Roles tab (settings/idp) points users there instead of
// editing assignments in place.
export const AgentDetailPage: React.FC = () => {
  const { orgId, envName, projectName, agentName } = useParams<{
    orgId: string;
    envName: string;
    projectName: string;
    agentName: string;
  }>();
  const [activeTab, setActiveTab] = useState<TabId>("roles");

  const { data: rolesData, isLoading: isLoadingRoles, error: rolesError } =
    useGetAgentRoles(
      { orgName: orgId, projName: projectName, agentName },
      { environment: envName ?? "" },
    );
  const { data: groupsData, isLoading: isLoadingGroups, error: groupsError } =
    useGetAgentGroups(
      { orgName: orgId, projName: projectName, agentName },
      { environment: envName ?? "" },
    );
  const { data: agentData, isLoading: isLoadingAgent } = useGetAgent({
    orgName: orgId,
    projName: projectName,
    agentName,
  });
  const { data: projectData, isLoading: isLoadingProject } = useGetProject({
    orgName: orgId,
    projName: projectName,
  });

  const roles = useMemo(() => rolesData?.roles ?? [], [rolesData]);
  const groups = useMemo(() => groupsData?.groups ?? [], [groupsData]);

  const agentsNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.view.children.agents;
  const agentsPath =
    orgId && envName ? generatePath(agentsNode.path, { orgId, envName }) : "#";

  const isLoading =
    isLoadingRoles || isLoadingGroups || isLoadingAgent || isLoadingProject;

  if (isLoading) {
    return (
      <>
        <BackButton to={agentsPath} label="Agents" />
        <EditFormSkeleton tabs={2} />
      </>
    );
  }

  return (
    <>
      <BackButton to={agentsPath} label="Agents" />
      <Stack spacing={3}>
        <EntityHeader
          fallback="A"
          name={agentData?.displayName || agentName || ""}
          subtitle={projectData?.displayName || projectName}
          id={agentName ?? ""}
        />

        {(rolesError != null || groupsError != null) && (
          <Alert severity="error">
            Failed to load this agent's roles/groups. Please try again.
          </Alert>
        )}

        <Form.Section>
          <Tabs
            value={activeTab}
            onChange={(_e, newValue) => setActiveTab(newValue as TabId)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Roles" value="roles" />
            <Tab label="Groups" value="groups" />
          </Tabs>

          {activeTab === "roles" && (
            <>
              <Form.Header>Assigned Roles</Form.Header>
              <Typography variant="body2" color="text.secondary">
                Roles assigned to this agent&apos;s identity in {envName}. To
                modify role assignments, use the Roles page.
              </Typography>

              <Box sx={{ mt: 1 }}>
                {roles.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    No roles assigned to this agent.
                  </Typography>
                ) : (
                  <Stack direction="row" flexWrap="wrap" gap={1}>
                    {roles.map((role) => (
                      <Chip key={role.id} label={role.name} size="small" />
                    ))}
                  </Stack>
                )}
              </Box>
            </>
          )}

          {activeTab === "groups" && (
            <>
              <Form.Header>Group Memberships</Form.Header>
              <Typography variant="body2" color="text.secondary">
                Groups this agent&apos;s identity belongs to in {envName}. To
                modify group memberships, use the Groups page.
              </Typography>

              <Box sx={{ mt: 1 }}>
                {groups.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    This agent is not a member of any groups.
                  </Typography>
                ) : (
                  <Stack direction="row" flexWrap="wrap" gap={1}>
                    {groups.map((group) => (
                      <Chip key={group.id} label={group.name} size="small" />
                    ))}
                  </Stack>
                )}
              </Box>
            </>
          )}
        </Form.Section>
      </Stack>
    </>
  );
};

export default AgentDetailPage;
