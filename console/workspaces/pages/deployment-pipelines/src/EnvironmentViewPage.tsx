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
  Alert,
  Avatar,
  Card,
  CardContent,
  Chip,
  Grid,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Copy, DoorClosedLocked, KeyRound } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  GatewayTypeChip,
  getAvatarInitials,
  getErrorMessage,
} from "@agent-management-platform/shared-component";
import {
  useGetEnvironment,
  useListGateways,
  useListThunderInstances,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type GatewayListenerSpec,
  type GatewayResponse,
  type GatewayStatus,
} from "@agent-management-platform/types";
import { PageLayout, useSnackBar } from "@agent-management-platform/views";

const monoEllipsisSx = {
  fontFamily: "monospace",
  color: "text.secondary",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  display: "block",
} as const;

const STATUS_COLOR: Record<GatewayStatus, "success" | "warning" | "error" | "default"> = {
  ACTIVE: "success",
  INACTIVE: "default",
  PROVISIONING: "warning",
  ERROR: "error",
};

const GATEWAY_AVATAR_SX = {
  width: 28,
  height: 28,
  fontSize: 12,
  bgcolor: "primary.main",
  color: "primary.contrastText",
} as const;

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
        <Stack spacing={0.5}>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
            {label}
          </Typography>
          <Typography
            variant="body2"
            sx={{ fontFamily: "monospace", wordBreak: "break-all" }}
          >
            {value}
          </Typography>
        </Stack>
      </CardContent>
    </Card>
  );
}

interface EndpointCardProps {
  label: string;
  scheme: "http" | "https";
  endpoint: GatewayListenerSpec;
  onCopy: (value: string, message: string) => void;
}

function EndpointCard({ label, scheme, endpoint, onCopy }: EndpointCardProps) {
  const url = `${scheme}://${endpoint.host}${endpoint.port ? `:${endpoint.port}` : ""}`;

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
              {url}
            </Typography>
            <Tooltip title={`Copy ${label}`}>
              <IconButton
                size="small"
                aria-label={`Copy ${label}`}
                onClick={() => onCopy(url, `${label} copied to clipboard`)}
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

export function EnvironmentViewPage() {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const navigate = useNavigate();
  const { pushSnackBar } = useSnackBar();

  const { data: env, isLoading: isLoadingEnv, error: envError } = useGetEnvironment({
    orgName: orgId,
    envName,
  });

  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgId },
    { environment: envName },
  );

  const backHref = orgId
    ? generatePath(absoluteRouteMap.children.org.children.environments.path, { orgId })
    : "#";

  const gateways = gatewaysData?.gateways ?? [];

  const { data: thunderInstancesData, isLoading: isLoadingProviders } =
    useListThunderInstances({ orgName: orgId });
  const thunderInstance = thunderInstancesData?.thunderInstances.find(
    (i) => i.envName === envName,
  );

  const handleCopy = (value: string, message: string) => {
    navigator.clipboard
      .writeText(value)
      .then(() => {
        pushSnackBar({ message, type: "success" });
      })
      .catch(() => {});
  };

  const httpEndpoint = env?.gateway?.ingress?.external?.http;
  const httpsEndpoint = env?.gateway?.ingress?.external?.https;

  const displayName = env?.displayName ?? env?.name ?? envName ?? "";

  return (
    <PageLayout
      title={displayName}
      backHref={backHref}
      backLabel="Back to Environments"
      description={
        env?.createdAt
          ? `Created ${formatDistanceToNow(new Date(env.createdAt), { addSuffix: true })}`
          : undefined
      }
      isLoading={isLoadingEnv}
      disableIcon
      titleTail={
        env ? (
          <Stack direction="row" alignItems="center" spacing={1}>
            <Chip
              label={env.isProduction ? "Production" : "Non-production"}
              size="small"
              variant="outlined"
              color={env.isProduction ? "primary" : "default"}
            />
            <Tooltip title="Data plane reference for this environment">
              <Chip label={env.dataplaneRef || "—"} size="small" variant="outlined" />
            </Tooltip>
          </Stack>
        ) : undefined
      }
    >
      {envError ? (
        <Alert severity="error" icon={<AlertTriangle size={18} />} sx={{ mb: 2 }}>
          {getErrorMessage(envError) || "Failed to load environment. Please try again."}
        </Alert>
      ) : null}

      {!isLoadingEnv && !env && !envError ? (
        <Alert severity="error" sx={{ mb: 2 }}>
          Environment &ldquo;{envName}&rdquo; not found.
        </Alert>
      ) : null}

      {isLoadingEnv && (
        <Stack spacing={3}>
          <Stack spacing={1.5}>
            <Skeleton variant="text" width={120} height={16} />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Skeleton variant="rounded" height={72} />
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Skeleton variant="rounded" height={72} />
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Skeleton variant="rounded" height={72} />
              </Grid>
            </Grid>
          </Stack>

          <Stack spacing={1.5}>
            <Skeleton variant="text" width={90} height={16} />
            <Stack spacing={1}>
              <Skeleton variant="rounded" height={56} />
              <Skeleton variant="rounded" height={56} />
            </Stack>
          </Stack>

          <Stack spacing={1.5}>
            <Skeleton variant="text" width={140} height={16} />
            <Skeleton variant="rounded" height={56} />
          </Stack>
        </Stack>
      )}

      {env && !envError && (
        <Stack spacing={3}>
          <Stack spacing={1.5}>
            {env.dnsPrefix || httpEndpoint || httpsEndpoint ? (
              <Grid container spacing={2}>
                {env.dnsPrefix && (
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <InfoCard label="DNS Prefix" value={env.dnsPrefix} />
                  </Grid>
                )}
                {httpEndpoint && (
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <EndpointCard
                      label="HTTP Endpoint"
                      scheme="http"
                      endpoint={httpEndpoint}
                      onCopy={handleCopy}
                    />
                  </Grid>
                )}
                {httpsEndpoint && (
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <EndpointCard
                      label="HTTPS Endpoint"
                      scheme="https"
                      endpoint={httpsEndpoint}
                      onCopy={handleCopy}
                    />
                  </Grid>
                )}
              </Grid>
            ) : (
              <Typography variant="body2" color="text.secondary">
                No external ingress configured for this environment.
              </Typography>
            )}
          </Stack>

          <Stack spacing={1.5}>
            <Typography variant="overline" color="text.secondary">
              Gateways
            </Typography>
            {isLoadingGateways ? (
              <Stack spacing={1}>
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} variant="rounded" height={56} />
                ))}
              </Stack>
            ) : gateways.length === 0 ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<DoorClosedLocked size={56} />}
                  title="No gateways in this environment"
                  description="Assign a gateway to this environment to see it here."
                />
              </ListingTable.Container>
            ) : (
              <ListingTable.Container>
                <ListingTable variant="table">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell>Gateway</ListingTable.Cell>
                      <ListingTable.Cell>Type</ListingTable.Cell>
                      <ListingTable.Cell>Virtual Host</ListingTable.Cell>
                      <ListingTable.Cell align="center">Status</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {gateways.map((gw: GatewayResponse) => (
                      <ListingTable.Row
                        key={gw.uuid}
                        variant="table"
                        hover
                        clickable
                        onClick={() =>
                          navigate(
                            generatePath(
                              absoluteRouteMap.children.org.children.gateways.children.view.path,
                              { orgId: orgId ?? "", gatewayId: gw.uuid },
                            ),
                          )
                        }
                      >
                        <ListingTable.Cell>
                          <ListingTable.CellIcon
                            icon={
                              <Avatar sx={GATEWAY_AVATAR_SX}>
                                {getAvatarInitials(gw.displayName ?? gw.name, {
                                  fallback: "G",
                                  maxChars: 1,
                                })}
                              </Avatar>
                            }
                            primary={gw.displayName ?? gw.name}
                          />
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <GatewayTypeChip type={gw.gatewayType} />
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                            {gw.vhost}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell align="center">
                          <Chip
                            label={gw.status}
                            size="small"
                            color={STATUS_COLOR[gw.status]}
                            variant="outlined"
                          />
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            )}
          </Stack>

          <Stack spacing={1.5}>
            <Typography variant="overline" color="text.secondary">
              Identity Providers
            </Typography>
            {isLoadingProviders ? (
              <Stack spacing={1}>
                <Skeleton variant="rounded" height={56} />
              </Stack>
            ) : !thunderInstance ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<KeyRound size={56} />}
                  title="No identity providers in this environment"
                  description="Each environment automatically gets a Thunder identity provider."
                />
              </ListingTable.Container>
            ) : (
              <ListingTable.Container>
                <ListingTable variant="table">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell width="240px">Identity Provider</ListingTable.Cell>
                      <ListingTable.Cell>Issuer</ListingTable.Cell>
                      <ListingTable.Cell>Token Endpoint</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    <ListingTable.Row
                      variant="table"
                      hover
                      clickable
                      onClick={() =>
                        navigate(
                          generatePath(
                            absoluteRouteMap.children.org.children.thunderInstances.children.view
                              .path,
                            { orgId: orgId ?? "", envName: thunderInstance.envName },
                          ),
                        )
                      }
                    >
                      <ListingTable.Cell>
                        <ListingTable.CellIcon
                          icon={
                            <Avatar
                              sx={{
                                width: 28,
                                height: 28,
                                bgcolor: "primary.main",
                                color: "primary.contrastText",
                              }}
                            >
                              <KeyRound size={16} />
                            </Avatar>
                          }
                          primary="Thunder"
                          secondary="System identity provider"
                        />
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="caption" sx={{ ...monoEllipsisSx, maxWidth: 280 }}>
                          {thunderInstance.issuerUrl}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Stack direction="row" alignItems="center" spacing={1}>
                          <Typography variant="caption" sx={{ ...monoEllipsisSx, maxWidth: 320 }}>
                            {thunderInstance.tokenUrl}
                          </Typography>
                          <Tooltip title="Copy token endpoint">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleCopy(
                                  thunderInstance.tokenUrl,
                                  "Token endpoint copied to clipboard",
                                );
                              }}
                            >
                              <Copy size={14} />
                            </IconButton>
                          </Tooltip>
                        </Stack>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            )}
          </Stack>
        </Stack>
      )}
    </PageLayout>
  );
}
