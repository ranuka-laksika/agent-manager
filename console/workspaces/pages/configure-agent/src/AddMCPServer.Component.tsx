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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  PageLayout,
  SelectionDrawer,
  SelectionIndicator,
  TextInput,
} from "@agent-management-platform/views";
import {
  Alert,
  Box,
  Button,
  Form,
  ListingTable,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Link, ServerCog } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type AgentMCPConfigResponse,
  type MCPProxyListItem,
} from "@agent-management-platform/types";
import {
  useCreateAgentMCPConfig,
  useGetAgent,
  useListAgentMCPConfigs,
  useListEnvironments,
  useListMCPProxies,
} from "@agent-management-platform/api-client";
import {
  ENV_VAR_KEYS,
  generateEnvVarNames,
  generateUniqueConfigName,
  type EnvVarKey,
} from "./utils/envConfig";

const ENV_VAR_DESCRIPTIONS: Record<EnvVarKey, string> = {
  url: "Base URL of the MCP server endpoint",
  apikey: "API key for authenticating with the MCP server endpoint",
};

function MCPServerDisplay({
  server,
  isSelected,
}: {
  server: MCPProxyListItem | null;
  isSelected: boolean;
}) {
  if (!server) return null;
  return (
    <Stack direction="row" spacing={2} flexGrow={1} alignItems="center">
      <SelectionIndicator selected={isSelected} />
      <Stack spacing={0.25} flexGrow={1}>
        <Typography variant="h6">{server.name}</Typography>
        {server.description && (
          <Typography variant="body2" color="text.secondary">
            {server.description}
          </Typography>
        )}
        <Stack direction="row" spacing={2}>
          {server.context && (
            <Typography variant="caption" color="text.secondary">
              Context: {server.context}
            </Typography>
          )}
          {server.version && (
            <Typography variant="caption" color="text.secondary">
              Version: {server.version}
            </Typography>
          )}
        </Stack>
      </Stack>
    </Stack>
  );
}

export const AddMCPServerComponent: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const navigate = useNavigate();

  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);
  const [serverByEnv, setServerByEnv] = useState<Record<string, string | null>>(
    {},
  );
  const [envVarNames, setEnvVarNames] = useState<Record<EnvVarKey, string>>(
    () => generateEnvVarNames(""),
  );
  const envVarNamesEditedRef = useRef(false);
  const [serverDrawerOpen, setServerDrawerOpen] = useState(false);

  const backHref =
    orgId && projectId && agentId
      ? generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.configure.path,
          { orgId, projectId, agentId },
        )
      : "#";

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternal = agent?.provisioning?.type === "external";

  const { data: environments = [], isLoading: isLoadingEnvironments } =
    useListEnvironments({ orgName: orgId });

  const { data: proxiesData, isLoading: isLoadingProxies } = useListMCPProxies(
    { orgName: orgId },
    { limit: 50, offset: 0 },
  );

  const { data: existingConfigsList } = useListAgentMCPConfigs(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { limit: 1000, offset: 0 },
  );

  const servers = useMemo(() => proxiesData?.list ?? [], [proxiesData]);
  const selectedEnvName = useMemo(
    () => environments[selectedEnvIndex]?.name ?? "",
    [environments, selectedEnvIndex],
  );
  const selectedServerId = selectedEnvName
    ? serverByEnv[selectedEnvName]
    : null;
  const selectedServer = useMemo(
    () => servers.find((s) => s.id === selectedServerId) ?? null,
    [servers, selectedServerId],
  );

  const primaryServerId = useMemo(() => {
    const firstEnvName = environments[0]?.name;
    return firstEnvName ? (serverByEnv[firstEnvName] ?? "") : "";
  }, [serverByEnv, environments]);

  useEffect(() => {
    if (envVarNamesEditedRef.current) return;
    setEnvVarNames(generateEnvVarNames(primaryServerId));
  }, [primaryServerId]);

  const createConfig = useCreateAgentMCPConfig();

  const handleSave = useCallback(() => {
    if (!orgId || !projectId || !agentId) return;

    const envMappings: Record<
      string,
      { proxyId?: string; configuration: Record<string, never> }
    > = {};
    let hasAtLeastOneServer = false;
    let resolvedProxyId = "";

    for (const env of environments) {
      const proxyId = serverByEnv[env.name] ?? null;
      if (!proxyId) continue;
      hasAtLeastOneServer = true;
      if (!resolvedProxyId) resolvedProxyId = proxyId;
      envMappings[env.name] = {
        proxyId,
        configuration: {},
      };
    }

    if (!hasAtLeastOneServer) return;

    const environmentVariables = !isExternal
      ? ENV_VAR_KEYS.map((key) => ({
          key,
          name: (envVarNames[key] ?? "").trim(),
        })).filter((envVar) => envVar.name.length > 0)
      : [];

    const existingNames = (existingConfigsList?.configs ?? []).map(
      (config) => config.name,
    );
    const name = generateUniqueConfigName(
      resolvedProxyId,
      "mcp",
      existingNames,
    );
    const body = {
      name,
      type: "mcp" as const,
      envMappings,
      environmentVariables:
        environmentVariables.length > 0 ? environmentVariables : undefined,
    };

    createConfig.mutate(
      {
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        body,
      },
      {
        onSuccess: (data: AgentMCPConfigResponse) => {
          const authInfoByEnv: Record<
            string,
            { type: string; in: string; name: string; value?: string }
          > = {};
          for (const [envName, mapping] of Object.entries(
            data.envMappings ?? {},
          )) {
            if (mapping.configuration?.authInfo) {
              authInfoByEnv[envName] = mapping.configuration.authInfo;
            }
          }
          navigate(
            generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents
                .children.configure.children.mcpProxies.children.view.path,
              { orgId, projectId, agentId, proxyId: data.uuid },
            ),
            {
              state: {
                authInfoByEnv:
                  Object.keys(authInfoByEnv).length > 0
                    ? authInfoByEnv
                    : undefined,
                openEnvPanel: true,
              },
            },
          );
        },
      },
    );
  }, [
    orgId,
    projectId,
    agentId,
    environments,
    serverByEnv,
    isExternal,
    envVarNames,
    existingConfigsList,
    createConfig,
    navigate,
  ]);

  const hasAnyServer = environments.some((env) => !!serverByEnv[env.name]);
  const isPending = createConfig.isPending;

  return (
    <PageLayout
      title="Add MCP Configuration"
      backHref={backHref}
      disableIcon
      backLabel="Back to Configure"
    >
      <Stack spacing={3}>
        {createConfig.isError ? (
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            onClose={createConfig.reset}
          >
            {String(
              createConfig.error instanceof Error
                ? createConfig.error.message
                : "Failed to create MCP configuration. Please try again.",
            )}
          </Alert>
        ) : null}

        <Form.Section>
          <Form.Header>MCP Server</Form.Header>
          {environments.length > 1 && !isLoadingEnvironments && (
            <>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                Select which MCP server to use in each environment.
              </Typography>
              <Tabs
                value={selectedEnvIndex}
                onChange={(_, value: number) => setSelectedEnvIndex(value)}
                sx={{ mb: 2 }}
              >
                {environments.map((env, index) => {
                  const hasServer = !!serverByEnv[env.name];
                  return (
                    <Tab
                      key={env.name}
                      label={
                        <Stack
                          direction="row"
                          spacing={0.5}
                          alignItems="center"
                        >
                          <span>{env.displayName ?? env.name}</span>
                          {!hasServer && (
                            <Tooltip
                              title="No MCP server selected"
                              placement="top"
                              arrow
                            >
                              <Box
                                component="span"
                                sx={{
                                  width: 8,
                                  height: 8,
                                  borderRadius: "50%",
                                  bgcolor: "warning.main",
                                  display: "inline-block",
                                }}
                              />
                            </Tooltip>
                          )}
                        </Stack>
                      }
                      value={index}
                    />
                  );
                })}
              </Tabs>
            </>
          )}

          {selectedServer ? (
            <Form.CardButton
              onClick={() => setServerDrawerOpen(true)}
              selected
              aria-label={`Selected: ${selectedServer.name}. Click to change.`}
            >
              <Form.CardContent>
                <MCPServerDisplay server={selectedServer} isSelected />
              </Form.CardContent>
            </Form.CardButton>
          ) : (
            <Box>
              {!isLoadingProxies && servers.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<ServerCog size={64} />}
                    title="No MCP servers available"
                    description="No MCP servers found. Add MCP servers from the organization MCP Proxies page first."
                    action={
                      orgId ? (
                        <Button
                          variant="contained"
                          size="small"
                          startIcon={<Link size={16} />}
                          onClick={() =>
                            navigate(
                              generatePath(
                                absoluteRouteMap.children.org.children
                                  .mcpProxies.children.add.path,
                                { orgId },
                              ),
                            )
                          }
                        >
                          Add MCP Server
                        </Button>
                      ) : undefined
                    }
                  />
                </ListingTable.Container>
              ) : (
                <Box sx={{ pt: 1 }}>
                  <Button
                    variant="outlined"
                    onClick={() => setServerDrawerOpen(true)}
                    disabled={
                      isLoadingProxies ||
                      servers.length === 0 ||
                      !selectedEnvName
                    }
                    startIcon={<Link size={16} />}
                  >
                    Select an MCP Server
                  </Button>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ display: "block", mt: 1 }}
                  >
                    Selecting a server will auto-generate environment variable
                    names below.
                  </Typography>
                </Box>
              )}
            </Box>
          )}

          <SelectionDrawer
            open={serverDrawerOpen}
            onClose={() => setServerDrawerOpen(false)}
            icon={<ServerCog size={24} />}
            title="Select MCP Server"
            description={
              environments.length > 1
                ? `Choose the MCP server for the ${environments[selectedEnvIndex]?.displayName ?? environments[selectedEnvIndex]?.name ?? ""} environment.`
                : "Choose the MCP server for this agent."
            }
            searchPlaceholder="Search MCP servers"
            items={servers}
            isLoading={isLoadingProxies}
            getItemKey={(server) => server.id ?? ""}
            isItemSelected={(server) => selectedServerId === server.id}
            matchesSearch={(server, query) =>
              (server.name ?? "").toLowerCase().includes(query) ||
              (server.description ?? "").toLowerCase().includes(query) ||
              (server.context ?? "").toLowerCase().includes(query)
            }
            onSelect={(server) => {
              if (selectedEnvName) {
                setServerByEnv((prev) => ({
                  ...prev,
                  [selectedEnvName]: server.id ?? null,
                }));
              }
            }}
            renderItem={(server, isSelected) => (
              <MCPServerDisplay server={server} isSelected={isSelected} />
            )}
            getItemAriaLabel={(server, isSelected) =>
              `${server.name}. ${isSelected ? "Selected" : "Click to select"}`
            }
            emptyState={{
              title: "No MCP servers available",
              description: "No MCP servers are available in the organization.",
            }}
            searchEmptyState={{
              title: "No MCP servers match your search",
              description:
                "Try a different keyword or clear the search filter.",
            }}
          />
        </Form.Section>

        {hasAnyServer && !isExternal && (
          <Form.Section>
            <Form.Header>Environment Variable Names</Form.Header>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              These names are shared across all environments. The platform
              injects the MCP server URL and API key values at runtime per
              environment. Edit only if your code uses different names.
            </Typography>
            <ListingTable.Container>
              <ListingTable density="compact">
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>
                      Variable Name{" "}
                      <Typography
                        component="span"
                        variant="caption"
                        color="text.secondary"
                      >
                        (editable)
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>Description</ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {ENV_VAR_KEYS.map((key) => (
                    <ListingTable.Row key={key}>
                      <ListingTable.Cell>
                        <TextInput
                          value={envVarNames[key] ?? ""}
                          onChange={(event) => {
                            envVarNamesEditedRef.current = true;
                            setEnvVarNames((prev) => ({
                              ...prev,
                              [key]: event.target.value,
                            }));
                          }}
                          copyable
                          copyTooltipText={`Copy ${envVarNames[key] || key}`}
                          size="small"
                        />
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="body2" color="text.secondary">
                          {ENV_VAR_DESCRIPTIONS[key]}
                        </Typography>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          </Form.Section>
        )}

        {isLoadingEnvironments ? (
          <Stack spacing={1}>
            <Skeleton variant="rounded" height={32} />
            <Skeleton variant="rounded" height={32} />
          </Stack>
        ) : null}

        <Box sx={{ display: "flex", gap: 1 }}>
          <Button variant="outlined" onClick={() => navigate(backHref)}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSave}
            disabled={!hasAnyServer || isPending}
          >
            {isPending ? "Saving..." : "Save"}
          </Button>
        </Box>
      </Stack>
    </PageLayout>
  );
};

export default AddMCPServerComponent;
