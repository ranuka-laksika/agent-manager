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

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  useCreateMCPProxyAPIKey,
  useListGateways,
  useRotateMCPProxyAPIKey,
} from "@agent-management-platform/api-client";
import type { MCPProxy } from "@agent-management-platform/types";
import {
  Alert,
  Button,
  Card,
  Chip,
  Divider,
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Copy, DoorClosedLocked, Key } from "@wso2/oxygen-ui-icons-react";
import { ACL_POLICY_NAME } from "../constants";

export type MCPProxyOverviewTabProps = {
  proxy: MCPProxy | null | undefined;
  orgName: string | undefined;
  isLoading?: boolean;
};

// Mirrors the gateway-side buildMCPProxyURL: {vhost}{context}/mcp.
function buildMCPInvokeUrl(vhost: string, context?: string): string {
  const base = vhost.startsWith("http") ? vhost : `https://${vhost}`;
  const trimmedBase = base.replace(/\/$/, "");
  const trimmedContext = context?.trim().replace(/^\/+|\/+$/g, "") ?? "";
  const path = trimmedContext ? `/${trimmedContext}/mcp` : "/mcp";
  return `${trimmedBase}${path}`;
}

export function MCPProxyOverviewTab({
  proxy,
  orgName,
  isLoading = false,
}: MCPProxyOverviewTabProps) {
  const theme = useTheme();

  const { data: gatewaysData } = useListGateways(
    { orgName: orgName ?? "" },
    { limit: 500 },
  );

  const gatewayOptions = useMemo(() => {
    const deployedGatewayIds = new Set(proxy?.gateways ?? []);
    const gateways = gatewaysData?.gateways ?? [];
    return gateways
      .filter((gateway) => deployedGatewayIds.has(gateway.uuid))
      .map((gateway) => ({
        uuid: gateway.uuid,
        url: buildMCPInvokeUrl(gateway.vhost, proxy?.context),
        displayName: gateway.displayName,
        name: gateway.name,
      }));
  }, [proxy?.gateways, proxy?.context, gatewaysData]);

  const [selectedGatewayId, setSelectedGatewayId] = useState("");
  const [generatedApiKey, setGeneratedApiKey] = useState<string | null>(null);
  const [apiKeyError, setApiKeyError] = useState<string | null>(null);
  const [invokeUrlCopied, setInvokeUrlCopied] = useState(false);
  const [apiKeyCopied, setApiKeyCopied] = useState(false);

  const selectedGateway = useMemo(
    () => gatewayOptions.find((gateway) => gateway.uuid === selectedGatewayId),
    [gatewayOptions, selectedGatewayId],
  );

  useEffect(() => {
    if (
      gatewayOptions.length > 0 &&
      (!selectedGatewayId ||
        !gatewayOptions.some((gateway) => gateway.uuid === selectedGatewayId))
    ) {
      setSelectedGatewayId(gatewayOptions[0].uuid);
    }
  }, [gatewayOptions, selectedGatewayId]);

  const handleCopyInvokeUrl = useCallback(async () => {
    if (!selectedGateway?.url) return;
    try {
      await navigator.clipboard.writeText(selectedGateway.url);
      setInvokeUrlCopied(true);
      setTimeout(() => setInvokeUrlCopied(false), 2000);
    } catch {
      // Silently fail
    }
  }, [selectedGateway?.url]);

  const createApiKey = useCreateMCPProxyAPIKey();
  const rotateApiKey = useRotateMCPProxyAPIKey();

  const isApiKeyConflictError = useCallback((err: unknown): boolean => {
    if (err && typeof err === "object") {
      const status =
        (err as { status?: number }).status ??
        (err as { statusCode?: number }).statusCode;
      if (status === 409) return true;
      const msg = String(
        (err as { message?: string }).message ??
          (err as { error?: string }).error ??
          "",
      ).toLowerCase();
      return (
        msg.includes("already exists") ||
        msg.includes("key exists") ||
        msg.includes("conflict")
      );
    }
    return false;
  }, []);

  const handleGenerateApiKey = useCallback(async () => {
    if (!orgName || !proxy?.id || !selectedGateway) return;
    setApiKeyError(null);
    setGeneratedApiKey(null);
    const randomSuffix = Math.random().toString(36).slice(2, 10);
    const keyName = `mcp-${proxy.id}-${randomSuffix}`;
    try {
      const res = await createApiKey.mutateAsync({
        params: { orgName, proxyId: proxy.id },
        body: {
          name: keyName,
          displayName: keyName,
        },
      });
      if (res.apiKey) setGeneratedApiKey(res.apiKey);
    } catch (createErr) {
      if (isApiKeyConflictError(createErr)) {
        try {
          const res = await rotateApiKey.mutateAsync({
            params: { orgName, proxyId: proxy.id, keyName },
            body: {},
          });
          if (res.apiKey) setGeneratedApiKey(res.apiKey);
        } catch (rotateErr) {
          setApiKeyError(
            rotateErr instanceof Error
              ? rotateErr.message
              : "Failed to rotate API key",
          );
        }
      } else {
        setApiKeyError(
          createErr instanceof Error
            ? createErr.message
            : "Failed to generate API key",
        );
      }
    }
  }, [
    createApiKey,
    isApiKeyConflictError,
    orgName,
    proxy?.id,
    rotateApiKey,
    selectedGateway,
  ]);

  if (isLoading) {
    return (
      <Stack spacing={2}>
        <Grid container spacing={2}>
          {[1, 2, 3, 4, 5].map((i) => (
            <Grid key={i} size={{ xs: 12, sm: 6, md: 4 }}>
              <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                <Stack spacing={1}>
                  <Skeleton variant="text" width="40%" height={16} />
                  <Skeleton variant="text" width="80%" height={20} />
                </Stack>
              </Card>
            </Grid>
          ))}
        </Grid>
      </Stack>
    );
  }

  if (!proxy) {
    return null;
  }

  return (
    <Stack spacing={3}>
      <Grid container spacing={2}>
        {proxy.context && (
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
                <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                  {proxy.context}
                </Typography>
              </Stack>
            </Card>
          </Grid>
        )}
        {proxy.upstream?.main?.url && (
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
              <Stack spacing={0.5}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 500 }}
                >
                  Upstream URL
                </Typography>
                <Typography
                  variant="body2"
                  sx={{ fontFamily: "monospace", wordBreak: "break-all" }}
                >
                  {proxy.upstream.main.url}
                </Typography>
              </Stack>
            </Card>
          </Grid>
        )}
        {proxy.upstream?.main?.auth?.type && (
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
              <Stack spacing={0.5}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ fontWeight: 500 }}
                >
                  Auth Type
                </Typography>
                <Typography variant="body2">
                  {proxy.upstream.main.auth.type}
                </Typography>
              </Stack>
            </Card>
          </Grid>
        )}
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
            <Stack spacing={0.5}>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ fontWeight: 500 }}
              >
                Access Control
              </Typography>
              <Chip
                label={
                  proxy.policies?.some((p) => p.name === ACL_POLICY_NAME)
                    ? "Configured"
                    : "Allow all"
                }
                size="small"
                variant="outlined"
                color={
                  proxy.policies?.some((p) => p.name === ACL_POLICY_NAME)
                    ? "success"
                    : "default"
                }
                sx={{ width: "fit-content" }}
              />
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

      {/* Invoke URL section */}
      <Stack spacing={2}>
        <Typography
          variant="subtitle2"
          color="text.secondary"
          sx={{ fontWeight: 600 }}
        >
          Invoke URL
        </Typography>
        {gatewayOptions.length > 0 ? (
          <Stack spacing={2}>
            <FormControl size="small" sx={{ minWidth: 200 }}>
              <FormLabel>Gateway</FormLabel>
              <Select
                value={selectedGatewayId || ""}
                onChange={(e) => {
                  setSelectedGatewayId(String(e.target.value ?? ""));
                  setGeneratedApiKey(null);
                  setApiKeyError(null);
                }}
                size="small"
                displayEmpty
              >
                {gatewayOptions.map((gateway) => (
                  <MenuItem key={gateway.uuid} value={gateway.uuid}>
                    <Stack direction="row" alignItems="center" gap={1}>
                      <DoorClosedLocked size={16} />
                      {gateway.displayName || gateway.name || gateway.uuid}
                    </Stack>
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            {selectedGateway && (
              <>
                <FormControl fullWidth size="small">
                  <FormLabel>Invoke URL</FormLabel>
                  <TextField
                    size="small"
                    fullWidth
                    key={selectedGatewayId}
                    value={selectedGateway.url}
                    slotProps={{
                      input: {
                        readOnly: true,
                        endAdornment: (
                          <InputAdornment position="end">
                            <Tooltip title={invokeUrlCopied ? "Copied!" : "Copy"}>
                              <IconButton
                                size="small"
                                onClick={handleCopyInvokeUrl}
                                aria-label="Copy Invoke URL"
                              >
                                <Copy size={16} />
                              </IconButton>
                            </Tooltip>
                          </InputAdornment>
                        ),
                      },
                    }}
                    sx={{
                      "& .MuiInputBase-input": {
                        fontFamily: "monospace",
                        fontSize: theme.typography.body2?.fontSize,
                        wordBreak: "break-all",
                      },
                    }}
                  />
                </FormControl>
                <Stack spacing={1}>
                  <FormLabel>Generate API Key</FormLabel>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Button
                      variant="outlined"
                      size="medium"
                      startIcon={<Key size={16} />}
                      onClick={handleGenerateApiKey}
                      disabled={createApiKey.isPending || rotateApiKey.isPending}
                    >
                      {createApiKey.isPending || rotateApiKey.isPending
                        ? "Generating..."
                        : "Generate API Key"}
                    </Button>
                  </Stack>
                  {apiKeyError && (
                    <Alert severity="error" onClose={() => setApiKeyError(null)}>
                      {apiKeyError}
                    </Alert>
                  )}
                  {generatedApiKey && (
                    <Alert
                      severity="success"
                      sx={{
                        "& .MuiAlert-message": {
                          flexGrow: 1,
                        },
                      }}
                    >
                      <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
                        API Key Generated
                      </Typography>
                      <Typography variant="body2" sx={{ mb: 1 }}>
                        Copy this API key now. It will not be shown again.
                      </Typography>
                      <Stack direction="row" spacing={1} flexGrow={1} alignItems="center">
                        <TextField
                          size="small"
                          fullWidth
                          value={generatedApiKey}
                          slotProps={{
                            input: {
                              readOnly: true,
                              endAdornment: (
                                <InputAdornment position="end">
                                  <Tooltip title={apiKeyCopied ? "Copied!" : "Copy"}>
                                    <IconButton
                                      size="small"
                                      onClick={async () => {
                                        try {
                                          await navigator.clipboard.writeText(
                                            generatedApiKey,
                                          );
                                          setApiKeyCopied(true);
                                          setTimeout(
                                            () => setApiKeyCopied(false),
                                            2000,
                                          );
                                        } catch {
                                          // Silently fail
                                        }
                                      }}
                                      aria-label="Copy API Key"
                                    >
                                      <Copy size={16} />
                                    </IconButton>
                                  </Tooltip>
                                </InputAdornment>
                              ),
                            },
                          }}
                          sx={{
                            "& .MuiInputBase-input": {
                              fontFamily: "monospace",
                              fontSize: theme.typography.body2?.fontSize,
                              wordBreak: "break-all",
                            },
                          }}
                        />
                      </Stack>
                    </Alert>
                  )}
                </Stack>
              </>
            )}
          </Stack>
        ) : (
          <Alert severity="info">
            No invoke URLs available. Deploy this MCP proxy to an AI gateway to
            see invoke URLs and generate API keys.
          </Alert>
        )}
      </Stack>
    </Stack>
  );
}

export default MCPProxyOverviewTab;
