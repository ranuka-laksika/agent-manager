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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  type SyntheticEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  useGetMCPProxy,
  useListEnvironments,
  useUpdateMCPProxy,
} from "@agent-management-platform/api-client";
import type {
  MCPEndpointConfig,
  MCPProxyEndpoint,
} from "@agent-management-platform/types";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  Chip,
  Divider,
  FormControl,
  IconButton,
  MenuItem,
  PageContent,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Clock,
  Edit,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import {
  formatRelativeTime,
  getAvatarInitials,
  normalizeVersion,
} from "@agent-management-platform/shared-component";
import { MCPCapabilitiesView } from "../components/MCPCapabilitiesView";
import { MCPProxyAccessControlTab } from "./MCPProxyAccessControlTab";
import { MCPProxyConnectionTab } from "./MCPProxyConnectionTab";
import { MCPProxyOverviewTab } from "./MCPProxyOverviewTab";
import { MCPProxyPoliciesTab } from "./MCPProxyPoliciesTab";
import { MCPProxyRewriteTab } from "./MCPProxyRewriteTab";
import { MCPProxySecurityTab } from "./MCPProxySecurityTab";
import { ManageEndpointsDialog } from "./ManageEndpointsDialog";

const TABS = [
  "Overview",
  "Capabilities",
  "Connection",
  "Access Control",
  "Security",
  "Rewrite",
  "Policies",
] as const;

// URL-safe stand-ins for each tab, index-aligned with TABS, so the selected
// tab (and environment, below) are shareable/deep-linkable and survive a
// page reload instead of resetting to Overview/first-environment.
const TAB_SLUGS = [
  "overview",
  "capabilities",
  "connection",
  "access-control",
  "security",
  "rewrite",
  "policies",
] as const;

export function ViewMCPProxy() {
  const { orgId, proxyId } = useParams<{ orgId: string; proxyId: string }>();
  const routeProxyId = proxyId ?? "";
  const [searchParams, setSearchParams] = useSearchParams();

  const tabSlug = searchParams.get("tab");
  const tabIndex = tabSlug
    ? Math.max(0, TAB_SLUGS.indexOf(tabSlug as (typeof TAB_SLUGS)[number]))
    : 0;
  const selectedEndpointId = searchParams.get("endpoint") ?? "";

  const setSelectedEndpointId = useCallback(
    (endpointId: string) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("endpoint", endpointId);
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const handleTabChange = useCallback(
    (_event: SyntheticEvent, value: number) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("tab", TAB_SLUGS[value]);
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const [isEditingDetails, setIsEditingDetails] = useState(false);
  const [manageEndpointsOpen, setManageEndpointsOpen] = useState(false);
  const [name, setName] = useState("");
  const [version, setVersion] = useState("");
  const [context, setContext] = useState("");
  const [description, setDescription] = useState("");
  const [baselineDetails, setBaselineDetails] = useState({
    context: "",
    description: "",
    id: "",
    name: "",
    version: "",
  });
  const {
    data: proxy,
    isLoading,
    error,
  } = useGetMCPProxy({
    orgName: orgId,
    proxyId: routeProxyId,
  });
  const { data: environments = [] } = useListEnvironments({
    orgName: orgId ?? "",
  });
  const updateMCPProxy = useUpdateMCPProxy();

  const endpoints = useMemo<MCPProxyEndpoint[]>(
    () => proxy?.endpoints ?? [],
    [proxy?.endpoints],
  );

  // Options for the endpoint dropdown, labelled with the endpoint name (falling back to
  // its handle).
  const endpointOptions = useMemo(() => {
    return endpoints.map((endpoint) => ({
      id: endpoint.id,
      label: endpoint.name || endpoint.id,
    }));
  }, [endpoints]);

  // Keep the selected endpoint valid: default to the first endpoint and reset when the
  // current selection is no longer present.
  useEffect(() => {
    if (endpoints.length === 0) {
      return;
    }
    if (!endpoints.some((endpoint) => endpoint.id === selectedEndpointId)) {
      setSelectedEndpointId(endpoints[0].id);
    }
  }, [endpoints, selectedEndpointId, setSelectedEndpointId]);

  const selectedEndpoint = useMemo<MCPProxyEndpoint | undefined>(
    () => endpoints.find((endpoint) => endpoint.id === selectedEndpointId),
    [endpoints, selectedEndpointId],
  );

  // The selected endpoint's flat config, consumed by every config tab. Environment
  // bindings and per-env deployment status are surfaced separately as chips.
  const selectedConfig = useMemo<MCPEndpointConfig | undefined>(
    () => selectedEndpoint,
    [selectedEndpoint],
  );

  // Chips describing each environment the selected endpoint is bound to, with its
  // per-environment deployment status.
  const selectedEnvChips = useMemo(() => {
    return (selectedEndpoint?.environments ?? []).map((binding) => {
      const env = environments.find(
        (item) => item.id === binding.environmentUuid,
      );
      return {
        id: binding.environmentUuid,
        label: env?.displayName ?? env?.name ?? binding.environmentUuid,
        status: binding.deploymentStatus,
      };
    });
  }, [selectedEndpoint, environments]);

  // Merge-and-save callback used by every config tab. It merges a partial into the
  // selected endpoint's flat config and PUTs the whole proxy with that one endpoint
  // replaced.
  const updateSelectedEndpointConfig = useCallback(
    async (fields: Partial<MCPEndpointConfig>) => {
      if (!orgId || !proxy?.id || !selectedEndpointId) {
        throw new Error("MCP proxy or endpoint is not loaded.");
      }
      const nextEndpoints = (proxy.endpoints ?? []).map((endpoint) =>
        endpoint.id === selectedEndpointId
          ? { ...endpoint, ...fields }
          : endpoint,
      );
      return updateMCPProxy.mutateAsync({
        params: { orgName: orgId, proxyId: proxy.id },
        body: { ...proxy, endpoints: nextEndpoints },
      });
    },
    [orgId, proxy, selectedEndpointId, updateMCPProxy],
  );

  const displayName = proxy?.name ?? proxy?.id ?? proxyId ?? "MCP Proxy";
  const hasUnsavedChanges =
    name !== baselineDetails.name ||
    version !== baselineDetails.version ||
    context !== baselineDetails.context ||
    description !== baselineDetails.description;
  const canSave = name.trim().length > 0 && version.trim().length > 0;

  useEffect(() => {
    const nextProxyId = proxy?.id ?? "";
    const isProxyChanged = nextProxyId !== baselineDetails.id;

    if (!isProxyChanged && isEditingDetails) {
      return;
    }

    const nextDetails = {
      context: proxy?.context ?? "",
      description: proxy?.description ?? "",
      id: nextProxyId,
      name: proxy?.name ?? "",
      version: proxy?.version ?? "",
    };
    setName(nextDetails.name);
    setVersion(nextDetails.version);
    setContext(nextDetails.context);
    setDescription(nextDetails.description);
    setBaselineDetails(nextDetails);
    setIsEditingDetails(false);
  }, [
    baselineDetails.id,
    isEditingDetails,
    proxy?.id,
    proxy?.name,
    proxy?.version,
    proxy?.context,
    proxy?.description,
  ]);

  const resetDraft = () => {
    setName(baselineDetails.name);
    setVersion(baselineDetails.version);
    setContext(baselineDetails.context);
    setDescription(baselineDetails.description);
    setIsEditingDetails(false);
  };

  const handleSave = async () => {
    if (!orgId || !proxy?.id) return;

    const updated = await updateMCPProxy.mutateAsync({
      params: { orgName: orgId, proxyId: proxy.id },
      body: {
        ...proxy,
        context: optionalString(context),
        description: optionalString(description),
        name: name.trim(),
        version: version.trim(),
      },
    });
    const nextDetails = {
      context: updated.context ?? "",
      description: updated.description ?? "",
      id: updated.id ?? "",
      name: updated.name ?? "",
      version: updated.version ?? "",
    };
    setName(nextDetails.name);
    setVersion(nextDetails.version);
    setContext(nextDetails.context);
    setDescription(nextDetails.description);
    setBaselineDetails(nextDetails);
    setIsEditingDetails(false);
  };

  if (isLoading) {
    return (
      <PageContent fullWidth>
        <Stack spacing={4}>
          <Skeleton variant="rounded" height={168} />
          <Skeleton variant="rounded" height={360} />
          <Skeleton variant="rounded" height={96} />
        </Stack>
      </PageContent>
    );
  }

  if (error) {
    return (
      <PageContent fullWidth>
        <Alert severity="error" icon={<AlertTriangle size={18} />}>
          {error instanceof Error
            ? error.message
            : "Failed to load MCP proxy. Please try again."}
        </Alert>
      </PageContent>
    );
  }

  const hasEndpoints = endpoints.length > 0;

  return (
    <PageContent fullWidth>
      <Stack spacing={4}>
        <Card variant="outlined" sx={{ p: 3 }}>
          <Stack
            direction={{ xs: "column", md: "row" }}
            spacing={3}
            justifyContent="space-between"
          >
            <Stack direction="row" spacing={3} alignItems="flex-start">
              <Avatar
                sx={{
                  bgcolor: "primary.main",
                  color: "primary.contrastText",
                  fontSize: 28,
                  fontWeight: 700,
                  height: 88,
                  width: 88,
                }}
              >
                {getAvatarInitials(displayName, { fallback: "MP" })}
              </Avatar>
              <Stack spacing={1}>
                <Stack direction="row" spacing={1} alignItems="center">
                  {isEditingDetails ? (
                    <TextField
                      label="Name"
                      size="small"
                      value={name}
                      onChange={(event) => setName(event.target.value)}
                      error={name.trim().length === 0}
                      helperText={
                        name.trim().length === 0
                          ? "Name is required."
                          : undefined
                      }
                      sx={{ minWidth: { xs: "100%", sm: 320 } }}
                    />
                  ) : (
                    <Typography variant="h4" fontWeight={500}>
                      {displayName}
                    </Typography>
                  )}
                  {isEditingDetails ? (
                    <TextField
                      label="Version"
                      size="small"
                      value={version}
                      onChange={(event) => setVersion(event.target.value)}
                      error={version.trim().length === 0}
                      helperText={
                        version.trim().length === 0
                          ? "Version is required."
                          : undefined
                      }
                      sx={{ minWidth: 160 }}
                    />
                  ) : proxy?.version ? (
                    <Chip
                      label={normalizeVersion(proxy.version)}
                      size="small"
                      variant="outlined"
                    />
                  ) : null}
                  <IconButton
                    size="small"
                    onClick={() => setIsEditingDetails(true)}
                    disabled={updateMCPProxy.isPending}
                    aria-label="Edit MCP proxy details"
                  >
                    <Edit size={18} />
                  </IconButton>
                </Stack>
                {isEditingDetails ? (
                  <Stack spacing={1.5} sx={{ maxWidth: 560 }}>
                    <TextField
                      label="Context"
                      size="small"
                      value={context}
                      onChange={(event) => setContext(event.target.value)}
                      placeholder="/default/my-mcp-proxy"
                    />
                    <TextField
                      label="Description"
                      size="small"
                      multiline
                      minRows={3}
                      value={description}
                      onChange={(event) => setDescription(event.target.value)}
                    />
                  </Stack>
                ) : (
                  <>
                    <Stack direction="row" spacing={2} alignItems="center">
                      <Typography variant="body2" color="text.secondary">
                        Context :
                      </Typography>
                      <Typography variant="body2">
                        {proxy?.context ?? "-"}
                      </Typography>
                    </Stack>
                    <Typography variant="body2" color="text.secondary">
                      {proxy?.description || "No description provided."}
                    </Typography>
                  </>
                )}
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography variant="body2" color="text.secondary">
                    Last updated :
                  </Typography>
                  <Clock size={16} />
                  <Typography variant="body2">
                    {formatRelativeTime(proxy?.updatedAt)}
                  </Typography>
                </Stack>
              </Stack>
            </Stack>
          </Stack>
        </Card>

        <Stack
          direction="row"
          spacing={2}
          alignItems="center"
          justifyContent="space-between"
        >
          {hasEndpoints ? (
            <Stack
              direction="row"
              spacing={2}
              alignItems="center"
              flexWrap="wrap"
              useFlexGap
            >
              <Typography variant="body2" color="text.secondary">
                Endpoint
              </Typography>
              <FormControl size="small" sx={{ minWidth: 260 }}>
                <Select
                  value={selectedEndpointId}
                  onChange={(event) =>
                    setSelectedEndpointId(event.target.value as string)
                  }
                >
                  {endpointOptions.map((option) => (
                    <MenuItem key={option.id} value={option.id}>
                      {option.label}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
              {selectedEnvChips.length > 0 ? (
                <Stack
                  direction="row"
                  spacing={1}
                  alignItems="center"
                  flexWrap="wrap"
                  useFlexGap
                >
                  {selectedEnvChips.map((chip) => (
                    <Chip
                      key={chip.id}
                      label={
                        chip.status ? `${chip.label} · ${chip.status}` : chip.label
                      }
                      size="small"
                      variant="outlined"
                      color={chip.status === "Deployed" ? "success" : "default"}
                    />
                  ))}
                </Stack>
              ) : null}
            </Stack>
          ) : (
            <span />
          )}
          <Button
            variant="outlined"
            startIcon={<Settings size={16} />}
            onClick={() => setManageEndpointsOpen(true)}
          >
            Manage Endpoints
          </Button>
        </Stack>

        {hasEndpoints ? (
          <Card variant="outlined">
            <Tabs value={tabIndex} onChange={handleTabChange}>
              {TABS.map((tab) => (
                <Tab key={tab} label={tab} />
              ))}
            </Tabs>
            <Divider />
            <Box sx={{ p: 3 }}>
              {tabIndex === 0 && (
                <MCPProxyOverviewTab
                  proxy={proxy}
                  config={selectedConfig}
                  environments={selectedEndpoint?.environments}
                  isLoading={isLoading}
                />
              )}
              {tabIndex === 1 && (
                <MCPCapabilitiesView
                  tools={selectedConfig?.capabilities?.tools}
                  resources={selectedConfig?.capabilities?.resources}
                  prompts={selectedConfig?.capabilities?.prompts}
                  sectionTitleVariant="h6"
                />
              )}
              {tabIndex === 2 && (
                <MCPProxyConnectionTab
                  config={selectedConfig}
                  selectedEndpointId={selectedEndpointId}
                  isLoading={isLoading}
                  onUpdate={updateSelectedEndpointConfig}
                  isUpdating={updateMCPProxy.isPending}
                />
              )}
              {tabIndex === 3 && (
                <MCPProxyAccessControlTab
                  config={selectedConfig}
                  selectedEndpointId={selectedEndpointId}
                  orgName={orgId}
                  isLoading={isLoading}
                  onUpdate={updateSelectedEndpointConfig}
                  isUpdating={updateMCPProxy.isPending}
                />
              )}
              {tabIndex === 4 && (
                <MCPProxySecurityTab
                  config={selectedConfig}
                  selectedEndpointId={selectedEndpointId}
                  isLoading={isLoading}
                  onUpdate={updateSelectedEndpointConfig}
                  isUpdating={updateMCPProxy.isPending}
                />
              )}
              {tabIndex === 5 && (
                <MCPProxyRewriteTab
                  config={selectedConfig}
                  selectedEndpointId={selectedEndpointId}
                  orgName={orgId}
                  isLoading={isLoading}
                  onUpdate={updateSelectedEndpointConfig}
                  isUpdating={updateMCPProxy.isPending}
                />
              )}
              {tabIndex === 6 && (
                <MCPProxyPoliciesTab
                  config={selectedConfig}
                  selectedEndpointId={selectedEndpointId}
                  orgName={orgId}
                  onUpdate={updateSelectedEndpointConfig}
                  isUpdating={updateMCPProxy.isPending}
                />
              )}
            </Box>
          </Card>
        ) : (
          <Card variant="outlined" sx={{ p: 3 }}>
            <Alert severity="info">
              This MCP proxy has no endpoints configured.
            </Alert>
          </Card>
        )}

        {hasUnsavedChanges ? (
          <Card variant="outlined" sx={{ p: 2 }}>
            <Stack direction="row" justifyContent="space-between" spacing={1}>
              <Typography variant="body2" color="warning.main" fontWeight={600}>
                You have unsaved changes.
              </Typography>
              <Stack direction="row" justifyContent="flex-end" spacing={1}>
                <Button
                  variant="outlined"
                  disabled={updateMCPProxy.isPending}
                  onClick={resetDraft}
                >
                  Cancel
                </Button>
                <Button
                  variant="contained"
                  disabled={!canSave || updateMCPProxy.isPending}
                  onClick={handleSave}
                >
                  {updateMCPProxy.isPending ? "Saving..." : "Save"}
                </Button>
              </Stack>
            </Stack>
          </Card>
        ) : null}
      </Stack>

      {proxy ? (
        <ManageEndpointsDialog
          open={manageEndpointsOpen}
          orgId={orgId ?? ""}
          proxy={proxy}
          environments={environments}
          onClose={() => setManageEndpointsOpen(false)}
        />
      ) : null}
    </PageContent>
  );
}

function optionalString(value: string) {
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

export default ViewMCPProxy;
