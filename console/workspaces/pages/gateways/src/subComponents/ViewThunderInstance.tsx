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

import React, { useCallback, useState } from "react";
import {
  Alert,
  Card,
  CardContent,
  Grid,
  IconButton,
  Skeleton,
  Snackbar,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, CheckCircle, Copy } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { useListThunderInstances } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";

function InfoCard({ label, value, monospace = false }: { label: string; value: string; monospace?: boolean }) {
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

function EndpointCard({ label, value, onCopy }: { label: string; value: string; onCopy: (v: string, l: string) => void }) {
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

export const ViewThunderInstance: React.FC = () => {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const [snackbarOpen, setSnackbarOpen] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState("");

  const { data, isLoading, error } = useListThunderInstances({ orgName: orgId });
  const instance = data?.thunderInstances.find((i) => i.envName === envName);

  const handleCopy = useCallback((value: string, label: string) => {
    navigator.clipboard.writeText(value).then(() => {
      setSnackbarMessage(`${label} copied to clipboard`);
      setSnackbarOpen(true);
    }).catch(() => {});
  }, []);

  const displayName = instance?.displayName || instance?.envName || envName || "";

  const curlSnippet = instance
    ? `curl -s -X POST "${instance.tokenUrl}" \\
  -H "Content-Type: application/x-www-form-urlencoded" \\
  -u "<client_id>:<client_secret>" \\
  -d "grant_type=client_credentials"`
    : "";

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.thunderInstances.path,
    { orgId: orgId ?? "" },
  );

  return (
    <>
      <PageLayout
        title={displayName}
        backHref={backHref}
        backLabel="Back to Identity"
        disableIcon
        isLoading={isLoading}
      >
        {isLoading && (
          <Stack spacing={3}>
            <Grid container spacing={2}>
              {[0, 1, 2, 3].map((i) => (
                <Grid key={i} size={{ xs: 12, sm: 6, md: 3 }}>
                  <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                    <Stack spacing={0.5}>
                      <Skeleton variant="text" width="40%" height={14} />
                      <Skeleton variant="text" width="85%" height={20} />
                    </Stack>
                  </Card>
                </Grid>
              ))}
            </Grid>
            <Card variant="outlined" sx={{ p: 3 }}>
              <Stack spacing={2}>
                <Skeleton variant="text" width={140} height={24} />
                <Skeleton variant="rounded" height={80} />
              </Stack>
            </Card>
          </Stack>
        )}

        {!!error && (
          <Alert severity="error" icon={<AlertTriangle size={18} />}>
            Failed to load identity provider. Please try again.
          </Alert>
        )}

        {!isLoading && !error && !instance && (
          <Alert severity="warning" icon={<AlertTriangle size={18} />}>
            Identity provider for environment &quot;{envName}&quot; was not found.
          </Alert>
        )}

        {instance && !error && (
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
                  <EndpointCard label="Token Endpoint" value={instance.tokenUrl} onCopy={handleCopy} />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                  <EndpointCard label="JWKS Endpoint" value={instance.jwksUrl} onCopy={handleCopy} />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                  <EndpointCard label="Issuer URL" value={instance.issuerUrl} onCopy={handleCopy} />
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

            {/* Quick Start */}
            <Card variant="outlined">
              <CardContent sx={{ p: 3 }}>
                <Typography variant="subtitle1" fontWeight={600} sx={{ mb: 1 }}>
                  Quick Start
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                  Mint a token using your agent&apos;s client credentials:
                </Typography>
                <TextField
                  fullWidth
                  multiline
                  minRows={4}
                  value={curlSnippet}
                  slotProps={{
                    input: {
                      readOnly: true,
                      endAdornment: (
                        <Tooltip title="Copy snippet">
                          <IconButton
                            size="small"
                            onClick={() => handleCopy(curlSnippet, "curl snippet")}
                            aria-label="Copy curl snippet"
                          >
                            <Copy size={16} />
                          </IconButton>
                        </Tooltip>
                      ),
                    },
                  }}
                  sx={{
                    "& .MuiInputBase-input": {
                      fontFamily: "monospace",
                      fontSize: "0.875rem",
                    },
                  }}
                />
              </CardContent>
            </Card>
          </Stack>
        )}
      </PageLayout>

      <Snackbar
        open={snackbarOpen}
        autoHideDuration={3000}
        onClose={() => setSnackbarOpen(false)}
        message={snackbarMessage}
      />
    </>
  );
};
