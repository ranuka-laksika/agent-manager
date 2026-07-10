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

import type {
  MCPEndpointConfig,
  MCPProxy,
} from "@agent-management-platform/types";
import { Card, Chip, Grid, IconButton, Skeleton, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Copy } from "@wso2/oxygen-ui-icons-react";
import { ACL_POLICY_NAME } from "../constants";
import { getAuthenticationTypeLabel, resolveAuthenticationType } from "./mcpEndpoints";
import { useCopyWithFeedback } from "./useCopyWithFeedback";

// One chip per environment the selected endpoint is bound to, with its deployment
// status — the same shape ViewMCPProxy already derives for the chips shown next to
// the endpoint selector, reused here instead of re-deriving a separate summary.
export type MCPProxyEnvironmentChip = {
  id: string;
  label: string;
  status?: "Deployed" | "Undeployed";
};

export type MCPProxyOverviewTabProps = {
  proxy: MCPProxy | null | undefined;
  config: MCPEndpointConfig | undefined;
  envChips?: MCPProxyEnvironmentChip[];
  isLoading?: boolean;
};

export function MCPProxyOverviewTab({
  proxy,
  config,
  envChips = [],
  isLoading = false,
}: MCPProxyOverviewTabProps) {
  const handleCopy = useCopyWithFeedback();

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

  const accessControlConfigured = config?.policies?.some(
    (policy) => policy.name === ACL_POLICY_NAME,
  );

  // Auth Type reflects the proxy's inbound security (the Security tab) — which
  // method clients must authenticate with — not the upstream auth used to reach
  // the backend.
  const authTypeLabel = getAuthenticationTypeLabel(
    resolveAuthenticationType(config),
  );

  const upstreamUrl = config?.upstream?.main?.url;

  return (
    <Stack spacing={3}>
      <Grid container spacing={2}>
        {upstreamUrl && (
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
                <Stack direction="row" alignItems="center" spacing={1}>
                  <Typography
                    variant="body2"
                    sx={{ fontFamily: "monospace", wordBreak: "break-all", flex: 1 }}
                  >
                    {upstreamUrl}
                  </Typography>
                  <Tooltip title="Copy Upstream URL">
                    <IconButton
                      size="small"
                      aria-label="Copy Upstream URL"
                      onClick={() => handleCopy(upstreamUrl, "Upstream URL")}
                    >
                      <Copy size={14} />
                    </IconButton>
                  </Tooltip>
                </Stack>
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
                Auth Type
              </Typography>
              <Typography variant="body2">{authTypeLabel}</Typography>
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
                Access Control
              </Typography>
              <Chip
                label={accessControlConfigured ? "Configured" : "Allow all"}
                size="small"
                variant="outlined"
                color={accessControlConfigured ? "success" : "default"}
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
                Deployment
              </Typography>
              {envChips.length === 0 ? (
                <Typography variant="body2" color="text.secondary">
                  No environments
                </Typography>
              ) : (
                <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                  {envChips.map((chip) => (
                    <Chip
                      key={chip.id}
                      label={chip.status ? `${chip.label} · ${chip.status}` : chip.label}
                      size="small"
                      variant="outlined"
                      color={chip.status === "Deployed" ? "success" : "default"}
                    />
                  ))}
                </Stack>
              )}
            </Stack>
          </Card>
        </Grid>
      </Grid>
    </Stack>
  );
}
