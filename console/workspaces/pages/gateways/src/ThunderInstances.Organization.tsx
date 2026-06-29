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

import { useState, useCallback } from "react";
import {
  Alert,
  Avatar,
  Box,
  Card,
  CardContent,
  Chip,
  Divider,
  Drawer,
  IconButton,
  ListingTable,
  Skeleton,
  Snackbar,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  CheckCircle,
  ChevronRight,
  Copy,
  KeyRound,
  X,
} from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
import { useListThunderInstances } from "@agent-management-platform/api-client";
import type { ThunderInstanceResponse } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";

function EndpointCard({
  label,
  value,
  onCopy,
}: {
  label: string;
  value: string;
  onCopy: (value: string, label: string) => void;
}) {
  return (
    <Card variant="outlined" sx={{ p: 0 }}>
      <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
        <Stack spacing={0.5}>
          <Typography variant="caption" color="text.secondary" fontWeight={500}>
            {label}
          </Typography>
          <Stack direction="row" alignItems="center" spacing={1}>
            <Typography
              variant="body2"
              sx={{
                fontFamily: "monospace",
                wordBreak: "break-all",
                flex: 1,
                fontSize: "0.8rem",
              }}
            >
              {value}
            </Typography>
            <Tooltip title={`Copy ${label}`}>
              <IconButton
                size="small"
                onClick={() => onCopy(value, label)}
                sx={{ flexShrink: 0 }}
              >
                <Copy size={14} />
              </IconButton>
            </Tooltip>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

function ThunderInstanceDrawer({
  instance,
  open,
  onClose,
  onCopy,
}: {
  instance: ThunderInstanceResponse | null;
  open: boolean;
  onClose: () => void;
  onCopy: (value: string, label: string) => void;
}) {
  if (!instance) return null;

  const displayName = instance.displayName || instance.envName;
  const curlSnippet = `curl -s -X POST "${instance.tokenUrl}" \\
  -H "Content-Type: application/x-www-form-urlencoded" \\
  -u "<client_id>:<client_secret>" \\
  -d "grant_type=client_credentials"`;

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{ sx: { width: { xs: "100%", sm: 520 }, p: 0 } }}
    >
      <Stack sx={{ height: "100%", overflow: "hidden" }}>
        {/* Header */}
        <Stack
          direction="row"
          alignItems="center"
          justifyContent="space-between"
          sx={{ px: 3, py: 2, borderBottom: "1px solid", borderColor: "divider" }}
        >
          <Stack direction="row" alignItems="center" spacing={1.5}>
            <Avatar
              sx={{
                bgcolor: "primary.main",
                color: "primary.contrastText",
                width: 36,
                height: 36,
                fontSize: 15,
              }}
            >
              {displayName.charAt(0).toUpperCase()}
            </Avatar>
            <Box>
              <Typography variant="subtitle1" fontWeight={600}>
                {displayName}
              </Typography>
              <Stack direction="row" spacing={0.5} alignItems="center">
                <Chip
                  label={instance.isProduction ? "Production" : "Non-production"}
                  size="small"
                  variant="outlined"
                  color={instance.isProduction ? "error" : "info"}
                />
              </Stack>
            </Box>
          </Stack>
          <IconButton size="small" onClick={onClose}>
            <X size={18} />
          </IconButton>
        </Stack>

        {/* Content */}
        <Box sx={{ flex: 1, overflowY: "auto", px: 3, py: 2.5 }}>
          <Stack spacing={3}>
            {/* Status */}
            <Stack direction="row" alignItems="center" spacing={1}>
              <CheckCircle size={16} color="var(--oxygen-palette-success-main)" />
              <Typography variant="body2" color="text.secondary">
                Thunder identity provider is active for this environment
              </Typography>
            </Stack>

            <Divider />

            {/* Endpoints */}
            <Stack spacing={1.5}>
              <Typography variant="subtitle2" fontWeight={600}>
                OAuth2 Endpoints
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Use these endpoints to configure your agent to mint and validate tokens.
              </Typography>
              <Alert severity="info" sx={{ py: 0.5 }}>
                These are cluster-internal addresses. Agents running inside the Kubernetes
                cluster can use them directly. For agents outside the cluster, expose Thunder
                through an ingress and use the public URL instead.
              </Alert>
              <EndpointCard
                label="Token Endpoint"
                value={instance.tokenUrl}
                onCopy={onCopy}
              />
              <EndpointCard
                label="JWKS Endpoint"
                value={instance.jwksUrl}
                onCopy={onCopy}
              />
              <EndpointCard
                label="Issuer URL"
                value={instance.issuerUrl}
                onCopy={onCopy}
              />
            </Stack>

            <Divider />

            {/* Infrastructure */}
            <Stack spacing={1.5}>
              <Typography variant="subtitle2" fontWeight={600}>
                Infrastructure
              </Typography>
              <EndpointCard
                label="Kubernetes Namespace"
                value={instance.namespace}
                onCopy={onCopy}
              />
            </Stack>

            <Divider />

            {/* Quick start */}
            <Stack spacing={1.5}>
              <Typography variant="subtitle2" fontWeight={600}>
                Quick Start
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Mint a token using your agent's client credentials:
              </Typography>
              <Card
                variant="outlined"
                sx={{ bgcolor: "action.hover", position: "relative" }}
              >
                <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
                  <Typography
                    component="pre"
                    variant="caption"
                    sx={{
                      fontFamily: "monospace",
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-all",
                      display: "block",
                      m: 0,
                    }}
                  >
                    {curlSnippet}
                  </Typography>
                  <Tooltip title="Copy snippet">
                    <IconButton
                      size="small"
                      onClick={() => onCopy(curlSnippet, "curl snippet")}
                      sx={{ position: "absolute", top: 8, right: 8 }}
                    >
                      <Copy size={14} />
                    </IconButton>
                  </Tooltip>
                </CardContent>
              </Card>
            </Stack>
          </Stack>
        </Box>
      </Stack>
    </Drawer>
  );
}

export function ThunderInstancesOrganization() {
  const { orgId } = useParams<{ orgId: string }>();
  const [selectedInstance, setSelectedInstance] =
    useState<ThunderInstanceResponse | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [snackbarOpen, setSnackbarOpen] = useState(false);
  const [snackbarMessage, setSnackbarMessage] = useState("");

  const {
    data,
    isLoading,
    error,
  } = useListThunderInstances({ orgName: orgId });

  const instances = data?.thunderInstances ?? [];

  const handleCopy = useCallback((value: string, label: string) => {
    navigator.clipboard.writeText(value).then(() => {
      setSnackbarMessage(`${label} copied to clipboard`);
      setSnackbarOpen(true);
    }).catch(() => {});
  }, []);

  const handleCardClick = useCallback((instance: ThunderInstanceResponse) => {
    setSelectedInstance(instance);
    setDrawerOpen(true);
  }, []);

  const handleDrawerClose = useCallback(() => {
    setDrawerOpen(false);
  }, []);

  const loadingSkeletons = (
    <ListingTable.Container disablePaper>
      <Stack spacing={1} mt={1}>
        {Array.from({ length: 3 }).map((_: unknown, i: number) => (
          <Stack
            key={i}
            direction="row"
            alignItems="center"
            spacing={2}
            sx={{
              px: 2,
              py: 1.5,
              borderRadius: 1,
              border: "1px solid",
              borderColor: "divider",
              bgcolor: "background.paper",
            }}
          >
            <Skeleton variant="circular" width={36} height={36} />
            <Skeleton variant="text" width={160} height={20} sx={{ flex: 1 }} />
            <Skeleton variant="rounded" width={96} height={24} />
            <Skeleton variant="rounded" width={24} height={24} />
          </Stack>
        ))}
      </Stack>
    </ListingTable.Container>
  );

  return (
    <>
      <PageLayout
        title="Identity"
        description="Environment-scoped OAuth2 identity providers for agent authentication"
        disableIcon
      >
        {isLoading && loadingSkeletons}

        {!!error && (
          <Alert severity="error" icon={<AlertTriangle size={18} />}>
            Failed to load identity providers. Please try again.
          </Alert>
        )}

        {!isLoading && !error && instances.length === 0 && (
          <ListingTable.Container>
            <ListingTable.EmptyState
              illustration={<KeyRound size={64} />}
              title="No identity providers"
              description="Add an environment first. Each environment automatically gets a Thunder OAuth2 identity provider."
            />
          </ListingTable.Container>
        )}

        {!isLoading && !error && instances.length > 0 && (
          <>
            {/* Instance list */}
            <ListingTable.Container disablePaper>
              <ListingTable variant="card">
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Environment</ListingTable.Cell>
                    <ListingTable.Cell>Token Endpoint</ListingTable.Cell>
                    <ListingTable.Cell width="48px" />
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {instances.map((instance: ThunderInstanceResponse) => (
                      <ListingTable.Row
                        key={instance.envName}
                        variant="card"
                        hover
                        clickable
                        onClick={() => handleCardClick(instance)}
                      >
                        <ListingTable.Cell>
                          <Stack direction="row" alignItems="center" spacing={2}>
                            <Avatar
                              sx={{
                                bgcolor: "primary.main",
                                color: "primary.contrastText",
                                fontSize: 15,
                                width: 36,
                                height: 36,
                                flexShrink: 0,
                              }}
                            >
                              {instance.envName.charAt(0).toUpperCase()}
                            </Avatar>
                            <Box>
                              <Typography variant="body2" fontWeight={500}>
                                {instance.envName} Environment Identity
                              </Typography>
                              <Typography
                                variant="caption"
                                color="text.secondary"
                                sx={{ fontFamily: "monospace" }}
                              >
                                {instance.envName}
                              </Typography>
                            </Box>
                          </Stack>
                        </ListingTable.Cell>

                        <ListingTable.Cell
                          onClick={(e) => e.stopPropagation()}
                        >
                          <Stack direction="row" alignItems="center" spacing={1}>
                            <Typography
                              variant="caption"
                              sx={{
                                fontFamily: "monospace",
                                color: "text.secondary",
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                                maxWidth: 320,
                                display: "block",
                              }}
                            >
                              {instance.tokenUrl}
                            </Typography>
                            <Tooltip title="Copy token endpoint">
                              <IconButton
                                size="small"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  handleCopy(
                                    instance.tokenUrl,
                                    "Token Endpoint",
                                  );
                                }}
                              >
                                <Copy size={14} />
                              </IconButton>
                            </Tooltip>
                          </Stack>
                        </ListingTable.Cell>

                        <ListingTable.Cell align="right">
                          <Tooltip title="View details">
                            <IconButton size="small">
                              <ChevronRight size={16} />
                            </IconButton>
                          </Tooltip>
                        </ListingTable.Cell>
                      </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          </>
        )}
      </PageLayout>

      <ThunderInstanceDrawer
        instance={selectedInstance}
        open={drawerOpen}
        onClose={handleDrawerClose}
        onCopy={handleCopy}
      />

      <Snackbar
        open={snackbarOpen}
        autoHideDuration={3000}
        onClose={() => setSnackbarOpen(false)}
        message={snackbarMessage}
      />
    </>
  );
}
