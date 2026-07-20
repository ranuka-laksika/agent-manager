/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import React, { useMemo, useState } from "react";
import { generatePath, useParams, useSearchParams } from "react-router-dom";
import { Box, Button, Card, Divider, Tab, Tabs } from "@wso2/oxygen-ui";
import { ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { usePipelineEnvironmentsState } from "@agent-management-platform/shared-component";
import {
  useDeleteAgentMCPConfig,
  useDeleteAgentModelConfig,
  useListAgentMCPConfigs,
  useListAgentModelConfigs,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  IDENTITY_ENV_PARAM,
  MANAGE_IDENTITY_PARAM,
} from "@agent-management-platform/types";
import {
  AgentConfigTableSection,
  type AgentConfigTableLabels,
} from "./Configure/subComponents/AgentConfigTableSection";
import { AddMCPToolConfigPanel } from "./Configure/subComponents/AddMCPToolConfigPanel";
import { ManageIdentityDrawer } from "./Configure/subComponents/ManageIdentityDrawer";
import { CONFIGURE_TAB_KEYS, CONFIGURE_TAB_PARAM } from "./configureTabs";

const configureRoutes =
  absoluteRouteMap.children.org.children.projects.children.agents.children
    .configure.children;

const llmLabels: AgentConfigTableLabels = {
  title: "LLM Configurations",
  searchPlaceholder: "Search LLM configurations...",
  addButtonLabel: "Add LLM Configuration",
  emptyTitle: "No LLM configurations added yet",
  emptyDescription:
    "Click Add LLM Configuration to connect a service provider.",
  errorTitle: "Failed to load LLM configurations",
  errorFallback: "Failed to load LLM configurations. Please try again.",
  searchEmptyTitle: "No LLM configurations match your search",
  searchEmptyDescription: "Try adjusting your search keywords.",
  removeTitle: "Remove LLM Configuration",
  removeTooltip: "Remove LLM configuration",
  removeConfirmation: () =>
    "This will remove the LLM configuration and its environment variable mappings from the agent. The catalog service itself will not be affected.",
  removeAriaLabel: (config) => `Remove configuration ${config.name || config.uuid}`,
};

const mcpLabels: AgentConfigTableLabels = {
  title: "Tool Configurations",
  searchPlaceholder: "Search by name or description...",
  addButtonLabel: "Add Tool Configuration",
  emptyTitle: "No tool configurations added yet",
  emptyDescription: "Add tool configurations that this agent can use.",
  errorTitle: "Failed to load tool configurations",
  errorFallback: "Failed to load tool configurations. Please try again.",
  searchEmptyTitle: "No tool configurations match your search criteria",
  searchEmptyDescription: "Try adjusting your search keywords.",
  removeTitle: "Remove Tool Configuration",
  removeTooltip: "Remove tool configuration",
  removeConfirmation: (config) =>
    `Are you sure you want to remove "${config.name}" from this agent?`,
  removeAriaLabel: (config) => `Remove ${config.name}`,
};

type TabPanelProps = {
  value: number;
  index: number;
  children: React.ReactNode;
};

function TabPanel({ value, index, children }: TabPanelProps) {
  return (
    <Box role="tabpanel" hidden={value !== index} sx={{ px: 3, py: 3 }}>
      {value === index ? children : null}
    </Box>
  );
}

export const ConfigureComponent: React.FC = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedTabIndex = CONFIGURE_TAB_KEYS.indexOf(
    searchParams.get(CONFIGURE_TAB_PARAM) as (typeof CONFIGURE_TAB_KEYS)[number],
  );
  const tabIndex = requestedTabIndex === -1 ? 0 : requestedTabIndex;
  const handleTabChange = (_: React.SyntheticEvent, index: number) => {
    // Preserve any other query params already on the URL instead of
    // replacing the whole search string with just this one.
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(CONFIGURE_TAB_PARAM, CONFIGURE_TAB_KEYS[index]);
        return next;
      },
      { replace: true },
    );
  };

  const [isAddingMcp, setIsAddingMcp] = useState(false);
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  // Both live in the URL (not component state) so a link from an
  // EnvironmentCard's "Manage AgentID" button can open the drawer already
  // pointed at the right environment, and so closing/reopening or reloading
  // doesn't lose the selection.
  const isManagingIdentity = searchParams.get(MANAGE_IDENTITY_PARAM) === "true";
  const selectedIdentityEnv = searchParams.get(IDENTITY_ENV_PARAM) ?? "";

  const openManageIdentity = () => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set(MANAGE_IDENTITY_PARAM, "true");
      return next;
    });
  };
  const closeManageIdentity = () => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.delete(MANAGE_IDENTITY_PARAM);
        next.delete(IDENTITY_ENV_PARAM);
        return next;
      },
      { replace: true },
    );
  };
  const setSelectedIdentityEnv = (envName: string) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(IDENTITY_ENV_PARAM, envName);
        return next;
      },
      { replace: true },
    );
  };

  const {
    environments: pipelineEnvs,
    isLoading: isPipelineEnvsLoading,
    isError: isPipelineEnvsError,
  } = usePipelineEnvironmentsState(orgId, projectId);
  const envNames = useMemo(() => pipelineEnvs.map((env) => env.name), [pipelineEnvs]);
  const { data: environmentsList = [] } = useListEnvironments({ orgName: orgId });
  const getEnvDisplayName = (name: string) =>
    environmentsList.find((env) => env.name === name)?.displayName ?? name;

  const {
    data: llmData,
    isLoading: isLoadingLLM,
    error: llmError,
  } = useListAgentModelConfigs(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { limit: 1000, offset: 0 },
  );
  const {
    data: mcpData,
    isLoading: isLoadingMCP,
    error: mcpError,
  } = useListAgentMCPConfigs(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { limit: 1000, offset: 0 },
  );
  const { mutate: deleteLLMConfig, isPending: isRemovingLLM } =
    useDeleteAgentModelConfig();
  const { mutate: deleteMCPConfig, isPending: isRemovingMCP } =
    useDeleteAgentMCPConfig();

  const llmConfigs = useMemo(() => llmData?.configs ?? [], [llmData]);
  const mcpConfigs = useMemo(() => mcpData?.configs ?? [], [mcpData]);

  const hasParams = Boolean(orgId && projectId && agentId);
  const deleteParams = {
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  };

  const llmAddPath = hasParams
    ? generatePath(configureRoutes.llmProviders.children.add.path, {
        orgId,
        projectId,
        agentId,
      })
    : "#";
  const getLlmViewPath = (configId: string) =>
    hasParams
      ? generatePath(configureRoutes.llmProviders.children.view.path, {
          orgId,
          projectId,
          agentId,
          configId: encodeURIComponent(configId),
        })
      : "#";
  const getMcpViewPath = (configId: string) =>
    hasParams
      ? generatePath(configureRoutes.mcpProxies.children.view.path, {
          orgId,
          projectId,
          agentId,
          proxyId: encodeURIComponent(configId),
        })
      : "#";

  return (
    <PageLayout
      title="Configure Agent"
      disableIcon
      actions={
        <Button
          variant="outlined"
          size="small"
          startIcon={<ShieldCheck size={16} />}
          onClick={openManageIdentity}
        >
          Manage AgentID
        </Button>
      }
    >
      <Card variant="outlined">
        <Tabs
          value={tabIndex}
          onChange={handleTabChange}
          variant="scrollable"
          allowScrollButtonsMobile
        >
          <Tab label={llmLabels.title} />
          <Tab label={mcpLabels.title} />
        </Tabs>
        <Divider />

        <TabPanel value={tabIndex} index={0}>
          <AgentConfigTableSection
            configs={llmConfigs}
            isLoading={isLoadingLLM}
            error={llmError}
            labels={llmLabels}
            addPath={llmAddPath}
            getViewPath={getLlmViewPath}
            isRemoving={isRemovingLLM}
            showTitle={false}
            onRemove={(configId) =>
              deleteLLMConfig({
                ...deleteParams,
                configId,
              })
            }
          />
        </TabPanel>

        <TabPanel value={tabIndex} index={1}>
          <AgentConfigTableSection
            configs={mcpConfigs}
            isLoading={isLoadingMCP}
            error={mcpError}
            labels={mcpLabels}
            onAdd={() => setIsAddingMcp(true)}
            getViewPath={getMcpViewPath}
            isRemoving={isRemovingMCP}
            showTitle={false}
            onRemove={(configId) =>
              deleteMCPConfig({
                ...deleteParams,
                configId,
              })
            }
          />
          {/* Single right-side drawer overlay; does not shrink the table. */}
          <AddMCPToolConfigPanel
            open={isAddingMcp}
            orgId={orgId}
            projectId={projectId}
            agentId={agentId}
            onClose={() => setIsAddingMcp(false)}
          />
        </TabPanel>
      </Card>

      <ManageIdentityDrawer
        open={isManagingIdentity}
        onClose={closeManageIdentity}
        orgId={orgId ?? ""}
        projectId={projectId ?? ""}
        agentId={agentId ?? ""}
        envNames={envNames}
        isEnvironmentsLoading={isPipelineEnvsLoading}
        isEnvironmentsError={isPipelineEnvsError}
        getEnvDisplayName={getEnvDisplayName}
        selectedEnvName={selectedIdentityEnv}
        onSelectedEnvNameChange={setSelectedIdentityEnv}
      />
    </PageLayout>
  );
};

export default ConfigureComponent;
