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
  MCPEnvironmentConfig,
  MCPProxy,
} from "@agent-management-platform/types";
import { Card, Chip, Grid, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import { ACL_POLICY_NAME } from "../constants";

export type MCPProxyOverviewTabProps = {
  proxy: MCPProxy | null | undefined;
  config: MCPEnvironmentConfig | undefined;
  isLoading?: boolean;
};

export function MCPProxyOverviewTab({
  proxy,
  config,
  isLoading = false,
}: MCPProxyOverviewTabProps) {
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

  // Whether this environment's single gateway artifact is currently deployed. The backend
  // computes it per environment from the artifact's deployment records.
  const isDeployed = config?.deploymentStatus === "Deployed";

  // Auth Type reflects the proxy's inbound security (the Security tab), i.e. whether
  // clients must present an API key — not the upstream auth used to reach the backend.
  const apiKeySecurityEnabled =
    config?.security?.enabled !== false &&
    !!config?.security?.apiKey &&
    config.security.apiKey.enabled !== false;

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
        {config?.upstream?.url && (
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
                  {config.upstream.url}
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
                Auth Type
              </Typography>
              <Typography variant="body2">
                {apiKeySecurityEnabled ? "API Key" : "None"}
              </Typography>
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
              <Chip
                label={isDeployed ? "Deployed" : "Undeployed"}
                size="small"
                variant="outlined"
                color={isDeployed ? "success" : "default"}
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
    </Stack>
  );
}

export default MCPProxyOverviewTab;
