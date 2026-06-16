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

import { useCallback, useEffect, useState } from "react";
import {
  useGetMCPProxy,
  useUpdateMCPProxy,
} from "@agent-management-platform/api-client";
import type {
  MCPProxy,
  MCPProxyPolicy,
} from "@agent-management-platform/types";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  Chip,
  Divider,
  IconButton,
  PageContent,
  Skeleton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Clock, Edit } from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
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

const TABS = [
  "Overview",
  "Capabilities",
  "Connection",
  "Access Control",
  "Security",
  "Rewrite",
  "Policies",
] as const;

export function ViewMCPProxy() {
  const { orgId, proxyId } = useParams<{ orgId: string; proxyId: string }>();
  const routeProxyId = proxyId ?? "";
  const [tabIndex, setTabIndex] = useState(0);
  const [isEditingDetails, setIsEditingDetails] = useState(false);
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
  const [editablePolicies, setEditablePolicies] = useState<MCPProxyPolicy[]>(
    [],
  );
  const [baselinePolicies, setBaselinePolicies] = useState<MCPProxyPolicy[]>(
    [],
  );
  const {
    data: proxy,
    isLoading,
    error,
  } = useGetMCPProxy({
    orgName: orgId,
    proxyId: routeProxyId,
  });
  const updateMCPProxy = useUpdateMCPProxy();

  // Merge-and-save callback used by self-contained tabs (e.g. Connection) that
  // persist a slice of the proxy independently of the details/policies form.
  const updateProxyFields = useCallback(
    async (fields: Partial<MCPProxy>) => {
      if (!orgId || !proxy?.id) {
        throw new Error("MCP proxy is not loaded.");
      }
      return updateMCPProxy.mutateAsync({
        params: { orgName: orgId, proxyId: proxy.id },
        body: { ...proxy, ...fields },
      });
    },
    [orgId, proxy, updateMCPProxy],
  );

  const displayName = proxy?.name ?? proxy?.id ?? proxyId ?? "MCP Proxy";
  const hasDetailsChanges =
    name !== baselineDetails.name ||
    version !== baselineDetails.version ||
    context !== baselineDetails.context ||
    description !== baselineDetails.description;
  const hasUnsavedChanges =
    hasDetailsChanges ||
    JSON.stringify(editablePolicies) !== JSON.stringify(baselinePolicies);
  const canSave = name.trim().length > 0 && version.trim().length > 0;

  useEffect(() => {
    const nextProxyId = proxy?.id ?? "";
    const isProxyChanged = nextProxyId !== baselineDetails.id;
    const hasPolicyDraftChanges =
      JSON.stringify(editablePolicies) !== JSON.stringify(baselinePolicies);

    if (!isProxyChanged && (isEditingDetails || hasPolicyDraftChanges)) {
      return;
    }

    const nextPolicies = proxy?.policies ?? [];
    setEditablePolicies(nextPolicies);
    setBaselinePolicies(nextPolicies);

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
    baselinePolicies,
    editablePolicies,
    isEditingDetails,
    proxy?.id,
    proxy?.policies,
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
    setEditablePolicies(baselinePolicies);
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
        policies: editablePolicies,
        version: version.trim(),
      },
    });
    setEditablePolicies(updated.policies ?? []);
    setBaselinePolicies(updated.policies ?? []);
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

        <Card variant="outlined">
          <Tabs
            value={tabIndex}
            onChange={(_, value: number) => setTabIndex(value)}
          >
            {TABS.map((tab) => (
              <Tab key={tab} label={tab} />
            ))}
          </Tabs>
          <Divider />
          <Box sx={{ p: 3 }}>
            {tabIndex === 0 && (
              <MCPProxyOverviewTab
                proxy={proxy}
                orgName={orgId}
                isLoading={isLoading}
              />
            )}
            {tabIndex === 1 && (
              <MCPCapabilitiesView
                tools={proxy?.capabilities?.tools}
                resources={proxy?.capabilities?.resources}
                prompts={proxy?.capabilities?.prompts}
                sectionTitleVariant="h6"
              />
            )}
            {tabIndex === 2 && (
              <MCPProxyConnectionTab
                proxy={proxy}
                isLoading={isLoading}
                onUpdate={updateProxyFields}
                isUpdating={updateMCPProxy.isPending}
              />
            )}
            {tabIndex === 3 && (
              <MCPProxyAccessControlTab
                proxy={proxy}
                orgName={orgId}
                isLoading={isLoading}
                onUpdate={updateProxyFields}
                isUpdating={updateMCPProxy.isPending}
              />
            )}
            {tabIndex === 4 && (
              <MCPProxySecurityTab
                proxy={proxy}
                isLoading={isLoading}
                onUpdate={updateProxyFields}
                isUpdating={updateMCPProxy.isPending}
              />
            )}
            {tabIndex === 5 && (
              <MCPProxyRewriteTab
                proxy={proxy}
                orgName={orgId}
                isLoading={isLoading}
                onUpdate={updateProxyFields}
                isUpdating={updateMCPProxy.isPending}
              />
            )}
            {tabIndex === 6 && (
              <MCPProxyPoliciesTab
                orgName={orgId}
                policies={editablePolicies}
                onPoliciesChange={setEditablePolicies}
              />
            )}
          </Box>
        </Card>

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
    </PageContent>
  );
}

function optionalString(value: string) {
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
}

export default ViewMCPProxy;
