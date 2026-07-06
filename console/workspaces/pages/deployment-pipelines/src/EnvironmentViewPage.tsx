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
  Card,
  Chip,
  Grid,
  ListingTable,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, DoorClosedLocked } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { GatewayTypeChip } from "@agent-management-platform/shared-component";
import {
  useListEnvironments,
  useListGateways,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap, type GatewayResponse, type GatewayStatus } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";

const STATUS_COLOR: Record<GatewayStatus, "success" | "warning" | "error" | "default"> = {
  ACTIVE: "success",
  INACTIVE: "default",
  PROVISIONING: "warning",
  ERROR: "error",
};

export function EnvironmentViewPage() {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const navigate = useNavigate();

  const { data: environments, isLoading: isLoadingEnv, error: envError } = useListEnvironments({
    orgName: orgId,
  });

  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgId },
    { environment: envName },
  );

  const backHref = orgId
    ? generatePath(absoluteRouteMap.children.org.children.environments.path, { orgId })
    : "#";

  const env = environments?.find((e) => e.name === envName);
  const gateways = gatewaysData?.gateways ?? [];

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
          <Chip
            label={env.isProduction ? "Production" : "Non-production"}
            size="small"
            variant="outlined"
            color={env.isProduction ? "primary" : "default"}
          />
        ) : undefined
      }
    >
      {envError ? (
        <Alert severity="error" icon={<AlertTriangle size={18} />} sx={{ mb: 2 }}>
          Failed to load environment. Please try again.
        </Alert>
      ) : null}

      {!isLoadingEnv && !env && !envError ? (
        <Alert severity="error" sx={{ mb: 2 }}>
          Environment &ldquo;{envName}&rdquo; not found.
        </Alert>
      ) : null}

      {env && !envError && (
        <Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 6, md: 4 }}>
              <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                <Stack spacing={0.5}>
                  <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                    Data Plane
                  </Typography>
                  <Typography variant="body2" sx={{ wordBreak: "break-all" }}>
                    {env.dataplaneRef || "—"}
                  </Typography>
                </Stack>
              </Card>
            </Grid>

            {env.dnsPrefix && (
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                      DNS Prefix
                    </Typography>
                    <Typography variant="body2" sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>
                      {env.dnsPrefix}
                    </Typography>
                  </Stack>
                </Card>
              </Grid>
            )}

            <Grid size={{ xs: 12, sm: 6, md: 4 }}>
              <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                <Stack spacing={0.5}>
                  <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                    Gateways
                  </Typography>
                  {isLoadingGateways ? (
                    <Stack direction="row" spacing={0.5}>
                      <Skeleton variant="rounded" width={70} height={24} />
                      <Skeleton variant="rounded" width={70} height={24} />
                    </Stack>
                  ) : gateways.length > 0 ? (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                      {gateways.map((gw: GatewayResponse) => (
                        <Chip
                          key={gw.uuid}
                          label={gw.displayName ?? gw.name}
                          size="small"
                          variant="outlined"
                        />
                      ))}
                    </Stack>
                  ) : (
                    <Typography variant="body2" color="text.secondary">
                      —
                    </Typography>
                  )}
                </Stack>
              </Card>
            </Grid>
          </Grid>

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
                            icon={undefined}
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
        </Stack>
      )}
    </PageLayout>
  );
}
