/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Alert, Card, CardContent, Grid, IconButton, Stack, Tooltip, Typography } from "@wso2/oxygen-ui";
import { CheckCircle, Copy } from "@wso2/oxygen-ui-icons-react";
import type { ThunderInstanceResponse } from "@agent-management-platform/types";

function InfoCard({
  label,
  value,
  monospace = false,
}: {
  label: string;
  value: string;
  monospace?: boolean;
}) {
  return (
    <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
      <Stack spacing={0.5}>
        <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
          {label}
        </Typography>
        <Typography
          variant="body2"
          sx={{ fontFamily: monospace ? "monospace" : undefined, wordBreak: "break-all" }}
        >
          {value}
        </Typography>
      </Stack>
    </Card>
  );
}

function EndpointCard({
  label,
  value,
  onCopy,
}: {
  label: string;
  value: string;
  onCopy: (v: string, l: string) => void;
}) {
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
        <Stack spacing={0.5}>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
            {label}
          </Typography>
          <Stack direction="row" alignItems="center" spacing={1}>
            <Typography
              variant="body2"
              sx={{ fontFamily: "monospace", wordBreak: "break-all", flex: 1, fontSize: "0.8rem" }}
            >
              {value}
            </Typography>
            <Tooltip title={`Copy ${label}`}>
              <IconButton size="small" onClick={() => onCopy(value, label)} sx={{ flexShrink: 0 }}>
                <Copy size={14} />
              </IconButton>
            </Tooltip>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

export type ThunderInstanceOverviewTabProps = {
  instance: ThunderInstanceResponse;
  onCopy: (value: string, label: string) => void;
};

export function ThunderInstanceOverviewTab({ instance, onCopy }: ThunderInstanceOverviewTabProps) {
  return (
    <Stack spacing={3}>
      {/* Status */}
      <Stack direction="row" alignItems="center" spacing={1}>
        <CheckCircle size={16} color="var(--oxygen-palette-success-main)" />
        <Typography variant="body2" color="text.secondary">
          Thunder identity provider is active for this environment
        </Typography>
      </Stack>

      {/* OAuth2 Endpoints */}
      <Stack spacing={1.5}>
        <Typography variant="subtitle2" fontWeight={600}>
          OAuth2 Endpoints
        </Typography>
        <Alert severity="info" sx={{ py: 0.5 }}>
          These are cluster-internal addresses. Agents running inside the Kubernetes cluster
          can use them directly. For agents outside the cluster, expose Thunder through an
          ingress and use the public URL instead.
        </Alert>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <EndpointCard label="Token Endpoint" value={instance.tokenUrl} onCopy={onCopy} />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <EndpointCard label="JWKS Endpoint" value={instance.jwksUrl} onCopy={onCopy} />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <EndpointCard label="Issuer URL" value={instance.issuerUrl} onCopy={onCopy} />
          </Grid>
        </Grid>
      </Stack>

      {/* Infrastructure */}
      <Stack spacing={1.5}>
        <Typography variant="subtitle2" fontWeight={600}>
          Infrastructure
        </Typography>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, sm: 6 }}>
            <InfoCard label="Kubernetes Namespace" value={instance.namespace} monospace />
          </Grid>
          <Grid size={{ xs: 12, sm: 6 }}>
            <InfoCard label="Environment" value={instance.envName} monospace />
          </Grid>
        </Grid>
      </Stack>
    </Stack>
  );
}

export default ThunderInstanceOverviewTab;
