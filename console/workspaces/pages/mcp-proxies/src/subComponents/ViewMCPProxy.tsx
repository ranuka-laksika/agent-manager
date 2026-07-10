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
import {
  absoluteRouteMap,
  type MCPEndpointConfig,
  type MCPProxy,
  type MCPProxyEndpoint,
} from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Divider,
  FormControl,
  Grid,
  IconButton,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Clock,
  Copy,
  Edit,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams, useSearchParams } from "react-router-dom";
import {
  formatRelativeTime,
  normalizeVersion,
} from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { MCPCapabilitiesView } from "../components/MCPCapabilitiesView";
import { MCPProxyAccessControlTab } from "./MCPProxyAccessControlTab";
import { MCPProxyConnectionTab } from "./MCPProxyConnectionTab";
import { MCPProxyOverviewTab } from "./MCPProxyOverviewTab";
import { MCPProxyPoliciesTab } from "./MCPProxyPoliciesTab";
import { MCPProxyRewriteTab } from "./MCPProxyRewriteTab";
import { MCPProxySecurityTab } from "./MCPProxySecurityTab";
import { EditMCPProxyDrawer } from "./EditMCPProxyDrawer";
import { ManageEndpointsDialog } from "./ManageEndpointsDialog";
import { useCopyWithFeedback } from "./useCopyWithFeedback";

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

  const [manageEndpointsOpen, setManageEndpointsOpen] = useState(false);
  const [editDrawerOpen, setEditDrawerOpen] = useState(false);
  const handleCopy = useCopyWithFeedback();

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
  const selectedConfig: MCPEndpointConfig | undefined = selectedEndpoint;

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
  const hasEndpoints = endpoints.length > 0;
  const backHref = generatePath(
    absoluteRouteMap.children.org.children.mcpProxies.path,
    { orgId: orgId ?? "" },
  );

  return (
    <>
      <PageLayout
        title={displayName}
        backHref={backHref}
        backLabel="Back to MCP Proxies"
        isLoading={isLoading}
        titleTail={
          proxy?.version ? (
            <Chip
              label={normalizeVersion(proxy.version)}
              size="small"
              variant="outlined"
              sx={{ ml: 1 }}
            />
          ) : undefined
        }
        description={proxy ? <MCPProxyDescription proxy={proxy} /> : undefined}
        actions={
          proxy ? (
            <Button
              variant="outlined"
              size="small"
              startIcon={<Edit size={16} />}
              onClick={() => setEditDrawerOpen(true)}
            >
              Edit Details
            </Button>
          ) : undefined
        }
      >
        {isLoading && (
          <Stack spacing={3}>
            <Skeleton variant="rounded" height={56} />
            <Skeleton variant="rounded" height={360} />
          </Stack>
        )}

        {error ? (
          <Alert severity="error" icon={<AlertTriangle size={18} />}>
            {error instanceof Error
              ? error.message
              : "Failed to load MCP proxy. Please try again."}
          </Alert>
        ) : null}

        {proxy && !error && (
          <Stack spacing={4}>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontWeight: 500 }}
                    >
                      Context
                    </Typography>
                    <Stack direction="row" alignItems="center" spacing={1}>
                      <Typography
                        variant="body2"
                        sx={{
                          fontFamily: "monospace",
                          wordBreak: "break-all",
                          flex: 1,
                        }}
                      >
                        {proxy.context || "—"}
                      </Typography>
                      {proxy.context && (
                        <Tooltip title="Copy Context">
                          <IconButton
                            size="small"
                            aria-label="Copy Context"
                            onClick={() =>
                              handleCopy(proxy.context as string, "Context")
                            }
                          >
                            <Copy size={14} />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Stack>
                  </Stack>
                </Card>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontWeight: 500 }}
                    >
                      In Catalog
                    </Typography>
                    <Chip
                      label={proxy.inCatalog ? "Yes" : "No"}
                      size="small"
                      color={proxy.inCatalog ? "success" : "default"}
                      variant="outlined"
                      sx={{ width: "fit-content" }}
                    />
                  </Stack>
                </Card>
              </Grid>
            </Grid>
            <Divider />
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              justifyContent="flex-end"
              flexWrap="wrap"
              useFlexGap
            >
              {hasEndpoints && (
                <FormControl size="small" sx={{ minWidth: 260 }}>
                  <Select
                    value={selectedEndpointId}
                    onChange={(event) =>
                      setSelectedEndpointId(event.target.value as string)
                    }
                    renderValue={(value) => {
                      const option = endpointOptions.find(
                        (o) => o.id === value,
                      );
                      return `${option?.label ?? value} Endpoint`;
                    }}
                  >
                    {endpointOptions.map((option) => (
                      <MenuItem key={option.id} value={option.id}>
                        {option.label}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              )}
              <Button
                variant="outlined"
                size="small"
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
                      envChips={selectedEnvChips}
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
                      orgName={orgId}
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
          </Stack>
        )}
      </PageLayout>

      {proxy && (
        <ManageEndpointsDialog
          open={manageEndpointsOpen}
          orgId={orgId ?? ""}
          proxy={proxy}
          environments={environments}
          onClose={() => setManageEndpointsOpen(false)}
        />
      )}
      {proxy && orgId && (
        <EditMCPProxyDrawer
          open={editDrawerOpen}
          onClose={() => setEditDrawerOpen(false)}
          proxy={proxy}
          orgId={orgId}
        />
      )}
    </>
  );
}

function MCPProxyDescription({ proxy }: { proxy: MCPProxy }) {
  return (
    <Stack spacing={0.75}>
      <Typography variant="body2" color="text.secondary">
        {proxy.description || "No description provided."}
      </Typography>
      {!proxy.description && (
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant="body2" color="text.secondary">
            Last updated:
          </Typography>
          <Clock size={16} />
          <Typography variant="body2">
            {formatRelativeTime(proxy.updatedAt)}
          </Typography>
        </Stack>
      )}
    </Stack>
  );
}

export default ViewMCPProxy;
