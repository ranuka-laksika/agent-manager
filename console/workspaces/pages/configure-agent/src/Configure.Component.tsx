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

import React, { useMemo } from "react";
import { Divider } from "@wso2/oxygen-ui";
import { generatePath, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import {
  useDeleteAgentMCPConfig,
  useDeleteAgentModelConfig,
  useListAgentMCPConfigs,
  useListAgentModelConfigs,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  AgentConfigTableSection,
  type AgentConfigTableLabels,
} from "./Configure/subComponents/AgentConfigTableSection";

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
  title: "MCP Configurations",
  searchPlaceholder: "Search by name or description...",
  addButtonLabel: "Add MCP Configuration",
  emptyTitle: "No MCP Configurations selected",
  emptyDescription: "Add MCP Configurations that this agent can use.",
  errorTitle: "Failed to load MCP configurations",
  errorFallback: "Failed to load MCP configurations. Please try again.",
  searchEmptyTitle: "No MCP Configurations match your search criteria",
  searchEmptyDescription: "Try adjusting your search keywords.",
  removeTitle: "Remove MCP Configuration",
  removeTooltip: "Remove MCP configuration",
  removeConfirmation: (config) =>
    `Are you sure you want to remove "${config.name}" from this agent?`,
  removeAriaLabel: (config) => `Remove ${config.name}`,
};

export const ConfigureComponent: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

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
  const { mutate: deleteLLMConfig } = useDeleteAgentModelConfig();
  const { mutate: deleteMCPConfig } = useDeleteAgentMCPConfig();

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
  const mcpAddPath = hasParams
    ? generatePath(configureRoutes.mcpProxies.children.add.path, {
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
          configId,
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
    <PageLayout title="Configure Agent" disableIcon>
      <AgentConfigTableSection
        configs={llmConfigs}
        isLoading={isLoadingLLM}
        error={llmError}
        labels={llmLabels}
        addPath={llmAddPath}
        getViewPath={getLlmViewPath}
        onRemove={(configId) =>
          deleteLLMConfig({
            ...deleteParams,
            configId,
          })
        }
      />
      <Divider sx={{ my: 3 }} />
      <AgentConfigTableSection
        configs={mcpConfigs}
        isLoading={isLoadingMCP}
        error={mcpError}
        labels={mcpLabels}
        addPath={mcpAddPath}
        getViewPath={getMcpViewPath}
        onRemove={(configId) =>
          deleteMCPConfig({
            ...deleteParams,
            configId,
          })
        }
      />
    </PageLayout>
  );
};

export default ConfigureComponent;
