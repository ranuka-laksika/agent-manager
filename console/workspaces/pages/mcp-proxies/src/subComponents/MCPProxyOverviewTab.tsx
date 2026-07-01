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

import { useCallback, useMemo, useState } from "react";
import {
  useCreateMCPProxyAPIKey,
  useListGateways,
  useRotateMCPProxyAPIKey,
} from "@agent-management-platform/api-client";
import type { MCPProxy } from "@agent-management-platform/types";
import {
  Card,
  Chip,
  Divider,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  InvokeEndpoints,
  type InvokeEndpoint,
} from "@agent-management-platform/shared-component";
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
  const { data: gatewaysData } = useListGateways(
    { orgName: orgName ?? "" },
    { limit: 500 },
  );

  const endpoints = useMemo<InvokeEndpoint[]>(() => {
    const deployedGatewayIds = new Set(proxy?.gateways ?? []);
    const gateways = gatewaysData?.gateways ?? [];
    return gateways
      .filter((gateway) => deployedGatewayIds.has(gateway.uuid))
      .map((gateway) => ({
        gatewayId: gateway.uuid,
        url: buildMCPInvokeUrl(gateway.vhost, proxy?.context),
        displayName: gateway.displayName,
        name: gateway.name,
        environment: gateway.environments?.[0],
      }));
  }, [proxy?.gateways, proxy?.context, gatewaysData]);

  const [generatedApiKey, setGeneratedApiKey] = useState<string | null>(null);
  const [apiKeyError, setApiKeyError] = useState<string | null>(null);

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
    if (!orgName || !proxy?.id || endpoints.length === 0) return;
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
    endpoints.length,
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

      <InvokeEndpoints
        endpoints={endpoints}
        onGenerateApiKey={handleGenerateApiKey}
        isGeneratingApiKey={createApiKey.isPending || rotateApiKey.isPending}
        apiKeyError={apiKeyError}
        generatedApiKey={generatedApiKey}
        onClearApiKeyError={() => setApiKeyError(null)}
        emptyMessage="No invoke URLs available. Deploy this MCP proxy to an AI gateway to see invoke URLs and generate API keys."
      />
    </Stack>
  );
}

export default MCPProxyOverviewTab;
