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

import { useEffect, useMemo, useState } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  PageLayout,
  SelectionDrawer,
  TextInput,
} from "@agent-management-platform/views";
import { CodeBlock } from "@agent-management-platform/shared-component";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Divider,
  Form,
  FormLabel,
  IconButton,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  BookOpen,
  Edit,
  ExternalLink,
  Link,
  ServerCog,
} from "@wso2/oxygen-ui-icons-react";
import {
  useGetAgent,
  useGetAgentMCPConfig,
  useGetMCPProxy,
  useListEnvironments,
  useListMCPProxies,
  useUpdateAgentMCPConfig,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type EnvironmentVariableConfig,
  type EnvProviderConfigMappings,
} from "@agent-management-platform/types";
import {
  generatePath,
  useLocation,
  useNavigate,
  useParams,
} from "react-router-dom";
import { MCPLogo } from "@agent-management-platform/mcp-proxies";
import { EnvironmentVariablesGuideDrawer } from "./Configure/subComponents/EnvironmentVariablesGuideDrawer";
import { MCPServerDisplay } from "./Configure/subComponents/MCPServerDisplay";
import { EmptyConfigCard } from "./Configure/subComponents/EmptyConfigCard";

type AuthInfoEntry = {
  type: string;
  in: string;
  name: string;
  value?: string;
};

export const ViewMCPServerComponent = () => {
  const { orgId, projectId, agentId, proxyId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    proxyId: string;
  }>();
  const decodedConfigId = useMemo(() => decodeRouteParam(proxyId), [proxyId]);
  const navigate = useNavigate();
  const location = useLocation();

  const authInfoByEnv = (
    location.state as {
      authInfoByEnv?: Record<string, AuthInfoEntry>;
      openEnvPanel?: boolean;
    } | null
  )?.authInfoByEnv;

  const [selectedEnvIndex, setSelectedEnvIndex] = useState(0);
  const [panelOpen, setPanelOpen] = useState(
    () => Boolean(
      (location.state as { openEnvPanel?: boolean } | null)?.openEnvPanel || authInfoByEnv,
    ),
  );
  const [envVarNames, setEnvVarNames] = useState<Record<string, string>>({});
  const [serverDrawerOpen, setServerDrawerOpen] = useState(false);
  // Pending MCP server change per env — set when the user picks in the drawer,
  // applied on save.
  const [pendingServerByEnv, setPendingServerByEnv] = useState<
    Record<string, string>
  >({});

  const {
    data: config,
    isLoading,
    isError,
    error,
  } = useGetAgentMCPConfig({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
    configId: decodedConfigId,
  });

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternal = agent?.provisioning?.type === "external";

  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const { data: proxiesData, isLoading: isLoadingProxies } = useListMCPProxies(
    { orgName: orgId },
    { limit: 50, offset: 0 },
  );
  const servers = useMemo(() => proxiesData?.list ?? [], [proxiesData]);
  const updateConfig = useUpdateAgentMCPConfig();

  const backHref =
    orgId && projectId && agentId
      ? generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
          .children.configure.path,
        { orgId, projectId, agentId },
      )
      : "#";

  const envNames = useMemo(() => {
    const configured = Object.keys(config?.envMappings ?? {});
    const ordered = environments
      .map((env) => env.name)
      .filter((name) => configured.includes(name));
    return ordered.length > 0 ? ordered : configured;
  }, [config, environments]);

  // Tabs and labels should show the human-friendly environment display name,
  // falling back to the raw name when no display name is set.
  const envDisplayName = (name: string) =>
    environments.find((e) => e.name === name)?.displayName ?? name;

  const selectedEnvName = envNames[selectedEnvIndex] ?? envNames[0] ?? "";
  const envMapping = config?.envMappings?.[selectedEnvName];
  const providerConfig = envMapping?.configuration;

  const sourceProxyName = getMCPProxyName(providerConfig);
  const sourceProxy = useMemo(
    () => (proxiesData?.list ?? []).find((proxy) => proxy.id === sourceProxyName),
    [proxiesData, sourceProxyName],
  );
  const { data: sourceProxyDetails } = useGetMCPProxy({
    orgName: orgId,
    proxyId: sourceProxyName ?? "",
  });
  const apiKeyHeaderName = getMCPAPIKeyHeaderName(sourceProxyDetails?.security);

  const envVarRows = useMemo<EnvironmentVariableConfig[]>(
    () => config?.environmentVariables ?? [],
    [config],
  );

  useEffect(() => {
    const nextNames: Record<string, string> = {};
    for (const envVar of envVarRows) {
      nextNames[envVar.key] = envVar.name;
    }
    setEnvVarNames(nextNames);
  }, [envVarRows]);

  const hasEmptyEnvVarName = envVarRows.some(
    (envVar) => (envVarNames[envVar.key] ?? envVar.name).trim() === "",
  );
  const isDirty = envVarRows.some(
    (envVar) => (envVarNames[envVar.key] ?? envVar.name) !== envVar.name,
  );
  // A pending selection that differs from the env's saved server is an edit.
  const proxyChangesDirty = Object.entries(pendingServerByEnv).some(
    ([envName, id]) =>
      id !== getMCPProxyName(config?.envMappings?.[envName]?.configuration),
  );

  const handleSave = () => {
    if (!orgId || !projectId || !agentId || !decodedConfigId || hasEmptyEnvVarName) {
      return;
    }

    // Only send envMappings when a server actually changed, so a plain env-var
    // rename never risks touching the existing per-env server mappings. When it
    // does change, send all envs (existing + newly picked) so none are dropped.
    let envMappings:
      | Record<string, { proxyId?: string; configuration: Record<string, never> }>
      | undefined;
    if (proxyChangesDirty) {
      envMappings = {};
      const editedEnvNames = new Set([
        ...Object.keys(config?.envMappings ?? {}),
        ...Object.keys(pendingServerByEnv),
      ]);
      for (const envName of editedEnvNames) {
        const existingId = getMCPProxyName(
          config?.envMappings?.[envName]?.configuration,
        );
        const resolvedId = pendingServerByEnv[envName] ?? existingId;
        if (!resolvedId) continue;
        envMappings[envName] = { proxyId: resolvedId, configuration: {} };
      }
    }

    updateConfig.mutate(
      {
        params: {
          orgName: orgId,
          projName: projectId,
          agentName: agentId,
          configId: decodedConfigId,
        },
        body: {
          environmentVariables: envVarRows.map((envVar) => ({
            key: envVar.key,
            name: (envVarNames[envVar.key] ?? envVar.name).trim(),
          })),
          ...(envMappings ? { envMappings } : {}),
        },
      },
      {
        onSuccess: () => {
          setPanelOpen(false);
          setPendingServerByEnv({});
        },
      },
    );
  };

  const resetEnvVarNames = () => {
    const nextNames: Record<string, string> = {};
    for (const envVar of envVarRows) {
      nextNames[envVar.key] = envVar.name;
    }
    setEnvVarNames(nextNames);
  };

  const envVarReferenceRows = useMemo(
    () =>
      envVarRows.map((envVar) => ({
        key: envVar.key,
        name: envVarNames[envVar.key] ?? envVar.name,
        description: describeMCPEnvVar(envVar.key),
      })),
    [envVarRows, envVarNames],
  );

  const pythonSnippet = useMemo(
    () => buildMCPPythonSnippet(envVarReferenceRows),
    [envVarReferenceRows],
  );

  if (isLoading) {
    return (
      <PageLayout
        title="MCP Configuration"
        backHref={backHref}
        disableIcon
        backLabel="Back to Configure"
      >
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={56} />
          <Skeleton variant="rounded" height={180} />
          <Skeleton variant="rounded" height={240} />
        </Stack>
      </PageLayout>
    );
  }

  if (isError || !config) {
    return (
      <PageLayout
        title="MCP Configuration"
        backHref={backHref}
        disableIcon
        backLabel="Back to Configure"
      >
        <Alert severity="error" icon={<AlertTriangle size={18} />}>
          {error instanceof Error
            ? error.message
            : "Configuration not found or failed to load."}
        </Alert>
      </PageLayout>
    );
  }

  const pageTitle = config.name || sourceProxy?.name || sourceProxyName;
  const showPanel = (isExternal && !!providerConfig)
    || (!isExternal && envVarRows.length > 0);

  const envVarsPanel = showPanel && (
    isExternal && providerConfig ? (
      <DrawerWrapper
        open={panelOpen}
        onClose={(_, reason) => {
          if (reason === "backdropClick") return;
          setPanelOpen(false);
        }}
        minWidth={640}
        maxWidth={640}
      >
        <DrawerHeader
          icon={<BookOpen size={24} />}
          title="Connect to MCP Server"
          onClose={() => setPanelOpen(false)}
        />
        <DrawerContent>
          {(() => {
            const authEntry = authInfoByEnv?.[selectedEnvName] ?? providerConfig.authInfo;
            const headerName = apiKeyHeaderName || authEntry?.name || "api-key";
            const headerValue = authEntry?.value || "<api-key>";
            const curlCode = [
              `curl -N ${providerConfig.url || "<endpoint-url>"}`,
              `  --header "${headerName}: ${headerValue}"`,
            ].join(" \\\n");
            return (
              <Stack spacing={2}>
                {authEntry?.value ? (
                  <>
                    <Alert severity="info">
                      <Typography variant="body2">
                        Configure your external agent with the endpoint and API key below to call
                        this MCP server through the gateway.
                      </Typography>
                    </Alert>
                    <Alert severity="warning">
                      <Typography variant="body2" fontWeight={600}>
                        Make sure to copy your API key now. You will not be able to see it again.
                      </Typography>
                    </Alert>
                  </>
                ) : (
                  <Alert severity="info">
                    <Typography variant="body2">
                      The endpoint is available below. If the MCP server requires an API key, the
                      key was only displayed when this configuration was created.
                    </Typography>
                  </Alert>
                )}
                {Boolean(providerConfig.url) && (
                  <TextInput
                    label="Endpoint URL"
                    value={providerConfig.url ?? ""}
                    copyable
                    copyTooltipText="Copy Endpoint URL"
                    slotProps={{ input: { readOnly: true } }}
                    size="small"
                  />
                )}
                <TextInput
                  label="Header Name"
                  value={headerName}
                  copyable
                  copyTooltipText="Copy Header Name"
                  slotProps={{ input: { readOnly: true } }}
                  size="small"
                />
                {authEntry?.value && (
                  <TextInput
                    label="API Key"
                    type="password"
                    value={authEntry.value}
                    copyable
                    copyTooltipText="Copy API Key"
                    slotProps={{ input: { readOnly: true } }}
                    size="small"
                  />
                )}
                <Box>
                  <FormLabel sx={{ display: "block", mb: 0.5 }}>Example cURL</FormLabel>
                  <CodeBlock code={curlCode} language="bash" fieldId="mcp-curl" />
                </Box>
              </Stack>
            );
          })()}
        </DrawerContent>
      </DrawerWrapper>
    ) : (
      <EnvironmentVariablesGuideDrawer
        open={panelOpen}
        onClose={() => setPanelOpen(false)}
        onCancel={() => {
          resetEnvVarNames();
          setPanelOpen(false);
        }}
        onSave={handleSave}
        isDirty={isDirty}
        isSaving={updateConfig.isPending}
        hasInvalidNames={hasEmptyEnvVarName}
        error={updateConfig.isError ? updateConfig.error : undefined}
        description={
          "These variable names are injected into the agent at runtime with environment-specific values. Rename them here if your code already uses different names, then save."
        }
        rows={envVarReferenceRows}
        onNameChange={(key, value) =>
          setEnvVarNames((prev) => ({
            ...prev,
            [key]: value,
          }))
        }
      >
        <Divider sx={{ my: 2 }} />
        <Stack spacing={1.5}>
          <Stack spacing={0.5}>
            <Typography variant="subtitle1" fontWeight={600}>
              Integration Guide
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Copy this pattern into your agent code to load MCP tools through the injected proxy
              URL and API key.
            </Typography>
          </Stack>
          <CodeBlock
            language="python"
            fieldId="mcp-python-snippet"
            code={pythonSnippet}
          />
        </Stack>
      </EnvironmentVariablesGuideDrawer>
    )
  );

  return (
    <PageLayout
      title={pageTitle}
      backHref={backHref}
      disableIcon
      backLabel="Back to Configuration Listing"
      actions={
        showPanel ? (
          <Button
            variant="outlined"
            size="small"
            startIcon={<BookOpen size={16} />}
            onClick={() => setPanelOpen(true)}
          >
            {isExternal ? "Connect to MCP Server" : "Environment Variables & Integration Guide"}
          </Button>
        ) : undefined
      }
    >
      <Stack spacing={3}>
        <Form.Section>
          <Form.Subheader>MCP Server</Form.Subheader>
          <Stack spacing={2.5}>
            {envNames.length > 1 && (
              <>
                <Typography variant="body2" color="text.secondary">
                  Each environment uses a separate MCP server mapping.
                </Typography>
                <Tabs
                  value={selectedEnvIndex}
                  onChange={(_, value: number) => setSelectedEnvIndex(value)}
                  sx={{ borderBottom: 1, borderColor: "divider", mb: 2 }}
                >
                  {envNames.map((envName, index) => (
                    <Tab
                      key={envName}
                      label={envDisplayName(envName)}
                      value={index}
                    />
                  ))}
                </Tabs>
              </>
            )}
            {(() => {
              const pendingId = pendingServerByEnv[selectedEnvName];
              const effectiveId = pendingId ?? sourceProxyName;
              const isPendingChange = !!pendingId && pendingId !== sourceProxyName;

              // No server mapped for this env and nothing picked yet — let the
              // user add one instead of showing an empty, non-actionable card.
              if (!effectiveId) {
                return (
                  <EmptyConfigCard
                    message="No MCP server is configured for this environment yet."
                    actionLabel="Select MCP Server"
                    actionIcon={<Link size={16} />}
                    onAction={() => setServerDrawerOpen(true)}
                    disabled={isLoadingProxies}
                  />
                );
              }

              const displayProxy = servers.find((s) => s.id === effectiveId);
              const proxyHref =
                orgId && effectiveId
                  ? generatePath(
                    absoluteRouteMap.children.org.children.mcpProxies.children
                      .view.path,
                    { orgId, proxyId: effectiveId },
                  )
                  : undefined;

              // The per-env URL is minted by the backend on save, so a pending
              // server change can't show a real URL/context yet.
              const contextValue =
                displayProxy?.context ??
                (isPendingChange ? "-" : getPathname(providerConfig?.url)) ??
                "Not configured";
              const envUrlValue = isPendingChange
                ? "Generated after saving"
                : (providerConfig?.url ?? "Not configured");
              const envUrlColor =
                !isPendingChange && providerConfig?.url
                  ? "text.primary"
                  : "text.disabled";

              return (
                <Card variant="outlined">
                  <CardContent sx={{ position: "relative" }}>
                    <Stack
                      direction="row"
                      spacing={0.5}
                      sx={{ position: "absolute", top: 8, right: 8 }}
                    >
                      <Tooltip title="Change MCP server" placement="top" arrow>
                        <IconButton
                          size="small"
                          color="primary"
                          onClick={() => setServerDrawerOpen(true)}
                          aria-label="Change MCP server"
                        >
                          <Edit size={16} />
                        </IconButton>
                      </Tooltip>
                      {proxyHref && (
                        <Tooltip title="View MCP proxy" placement="top" arrow>
                          <IconButton
                            size="small"
                            color="primary"
                            onClick={() => navigate(proxyHref)}
                            aria-label={`View MCP proxy ${displayProxy?.name ?? effectiveId} in the organization`}
                          >
                            <ExternalLink size={16} />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Stack>
                    <Stack
                      direction="row"
                      spacing={2}
                      flexGrow={1}
                      alignItems="flex-start"
                    >
                      <Avatar
                        sx={{
                          height: 36,
                          width: 36,
                          backgroundColor: "action.selected",
                        }}
                      >
                        <Box sx={{ color: "text.secondary", display: "inline-flex" }}>
                          <MCPLogo size={20} />
                        </Box>
                      </Avatar>
                      <Stack spacing={0.5} flexGrow={1} sx={{ minWidth: 0 }}>
                        <Stack
                          direction="row"
                          spacing={0.75}
                          alignItems="center"
                          flexWrap="wrap"
                          useFlexGap
                          sx={{ minHeight: 36 }}
                        >
                          <Typography variant="h6">
                            {displayProxy?.name ?? effectiveId ?? config.name}
                          </Typography>
                          {displayProxy?.version && (
                            <Chip
                              label={displayProxy.version}
                              size="small"
                              variant="outlined"
                            />
                          )}
                        </Stack>
                        <Typography variant="caption" color="text.secondary">
                          Context:{" "}
                          <Typography
                            component="span"
                            variant="caption"
                            color={
                              displayProxy?.context
                                ? "text.primary"
                                : "text.disabled"
                            }
                          >
                            {contextValue}
                          </Typography>
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          Environment URL:{" "}
                          <Typography
                            component="span"
                            variant="caption"
                            color={envUrlColor}
                            sx={{ wordBreak: "break-all" }}
                          >
                            {envUrlValue}
                          </Typography>
                        </Typography>
                      </Stack>
                    </Stack>
                  </CardContent>
                </Card>
              );
            })()}

            {proxyChangesDirty && (
              <Stack direction="row" spacing={1} justifyContent="flex-end">
                <Button
                  variant="outlined"
                  size="small"
                  onClick={() => setPendingServerByEnv({})}
                >
                  Cancel
                </Button>
                <Button
                  variant="contained"
                  size="small"
                  onClick={handleSave}
                  disabled={updateConfig.isPending}
                >
                  {updateConfig.isPending ? "Saving…" : "Save"}
                </Button>
              </Stack>
            )}

            <SelectionDrawer
              open={serverDrawerOpen}
              onClose={() => setServerDrawerOpen(false)}
              icon={<ServerCog size={24} />}
              title="Select MCP Server"
              description={
                envNames.length > 1
                  ? `Choose the MCP server for the ${envDisplayName(selectedEnvName)} environment.`
                  : "Choose the MCP server for this agent."
              }
              searchPlaceholder="Search MCP servers"
              items={servers}
              isLoading={isLoadingProxies}
              getItemKey={(server) => server.id ?? ""}
              isItemSelected={(server) =>
                (pendingServerByEnv[selectedEnvName] ?? sourceProxyName) ===
                server.id
              }
              matchesSearch={(server, query) =>
                (server.name ?? "").toLowerCase().includes(query) ||
                (server.description ?? "").toLowerCase().includes(query) ||
                (server.context ?? "").toLowerCase().includes(query)
              }
              onSelect={(server) => {
                if (selectedEnvName && server.id) {
                  setPendingServerByEnv((prev) => ({
                    ...prev,
                    [selectedEnvName]: server.id as string,
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
                description:
                  "No MCP servers are available in the organization.",
              }}
              searchEmptyState={{
                title: "No MCP servers match your search",
                description:
                  "Try a different keyword or clear the search filter.",
              }}
            />
          </Stack>
        </Form.Section>
      </Stack>

      {envVarsPanel}
    </PageLayout>
  );
};

function decodeRouteParam(value?: string) {
  if (!value) return "";
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function getMCPProxyName(config?: EnvProviderConfigMappings["configuration"]): string | undefined {
  return (
    config?.proxyName ??
    config?.proxyId ??
    config?.mcpProxyName ??
    config?.mcpProxyId ??
    config?.providerName
  );
}

function getMCPAPIKeyHeaderName(
  security?: { enabled?: boolean; apiKey?: { enabled?: boolean; key?: string } },
): string | undefined {
  if (security?.enabled === false || security?.apiKey?.enabled === false) {
    return undefined;
  }
  const headerName = security?.apiKey?.key?.trim();
  return headerName || "X-API-Key";
}

function getPathname(value?: string) {
  if (!value) return undefined;
  try {
    return new URL(value).pathname;
  } catch {
    return value;
  }
}

function describeMCPEnvVar(key: string): string {
  if (/url/i.test(key)) return "Base URL of the MCP server endpoint";
  if (/api[-_]?key/i.test(key)) return "API key for authenticating with the MCP server endpoint";
  return key.replace(/([A-Z])/g, " $1").replace(/^./, (str) => str.toUpperCase());
}

function buildMCPPythonSnippet(rows: { key: string; name: string }[]): string {
  const urlEnvVar = rows.find((row) => /url/i.test(row.key))?.name ?? "MCP_SERVER_URL";
  const apiKeyEnvVar =
    rows.find((row) => /api[-_]?key/i.test(row.key))?.name ?? "MCP_SERVER_API_KEY";

  return [
    "import os",
    "from typing import Any",
    "from langchain_mcp_adapters.client import MultiServerMCPClient",
    "",
    `raw_urls = os.environ.get("${urlEnvVar}", "")`,
    "mcp_server_urls = [url.strip() for url in raw_urls.split(\",\") if url.strip()]",
    `mcp_api_key = os.environ.get("${apiKeyEnvVar}", "").strip()`,
    "",
    "server_configs: dict[str, dict[str, Any]] = {",
    "    f\"mcp_server_{i}\": {",
    "        \"url\": url,",
    "        \"transport\": \"streamable_http\",",
    "        \"headers\": {",
    "            \"API-Key\": mcp_api_key,",
    "            \"Authorization\": \"\",",
    "        },",
    "    }",
    "    for i, url in enumerate(mcp_server_urls)",
    "} if mcp_server_urls and mcp_api_key else {}",
    "",
    "mcp_client = MultiServerMCPClient(server_configs)",
    "tools = await mcp_client.get_tools()",
  ].join("\n");
}

export default ViewMCPServerComponent;
