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

import { useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, KeyRound, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { useParams } from "react-router-dom";
import { useListIdentityProviders } from "@agent-management-platform/api-client";
import type { IdentityProvider } from "@agent-management-platform/types";
import {
  ManageIdentityProviderDialog,
  type ManageIdentityProviderMode,
} from "./ManageIdentityProviderDialog";

export function IdentityProvidersTable() {
  const { orgId } = useParams<{ orgId: string }>();
  const [hoveredKey, setHoveredKey] = useState<string | null>(null);
  const [dialogMode, setDialogMode] = useState<ManageIdentityProviderMode>("upsert");
  const [dialogProvider, setDialogProvider] = useState<IdentityProvider | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  const {
    data: providersResp,
    isLoading,
    error,
  } = useListIdentityProviders({ orgName: orgId });

  const providers = useMemo(() => providersResp?.list ?? [], [providersResp]);

  const openAdd = () => {
    setDialogMode("upsert");
    setDialogProvider(null);
    setDialogOpen(true);
  };

  const openDelete = (provider: IdentityProvider) => {
    setDialogMode("delete");
    setDialogProvider(provider);
    setDialogOpen(true);
  };

  // A provider row is uniquely keyed by gateway + name (a provider may appear
  // once per environment its gateway maps to).
  const rowKey = (p: IdentityProvider) => `${p.gatewayId ?? ""}:${p.name}`;

  const addButton = (
    <Button
      variant="contained"
      color="primary"
      startIcon={<Plus size={16} />}
      onClick={openAdd}
    >
      Add Identity Provider
    </Button>
  );

  const dialog = (
    <ManageIdentityProviderDialog
      open={dialogOpen}
      onClose={() => setDialogOpen(false)}
      orgId={orgId ?? ""}
      mode={dialogMode}
      provider={dialogProvider}
    />
  );

  if (error) {
    return (
      <Stack spacing={1}>
        <Box display="flex" justifyContent="flex-end">{addButton}</Box>
        <ListingTable.Container>
          <Alert severity="error" icon={<AlertTriangle size={18} />} sx={{ alignSelf: "stretch" }}>
            {error instanceof Error
              ? error.message
              : "Failed to load identity providers. Please try again."}
          </Alert>
        </ListingTable.Container>
        {dialog}
      </Stack>
    );
  }

  if (isLoading) {
    return (
      <Stack spacing={1}>
        <Box display="flex" justifyContent="flex-end">{addButton}</Box>
        <ListingTable.Container disablePaper>
          <Stack spacing={1} mt={1}>
            {Array.from({ length: 3 }).map((_, i) => (
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
                <Skeleton variant="text" width={160} height={20} />
                <Skeleton variant="rounded" width={72} height={24} sx={{ flexShrink: 0 }} />
                <Skeleton variant="text" sx={{ flex: 1 }} height={18} />
                <Skeleton variant="text" width={100} height={18} sx={{ flexShrink: 0, ml: "auto" }} />
              </Stack>
            ))}
          </Stack>
        </ListingTable.Container>
        {dialog}
      </Stack>
    );
  }

  if (!providers.length) {
    return (
      <Stack spacing={1}>
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<KeyRound size={64} />}
            title="No identity providers configured"
            description="Add an identity provider (token issuer) so agents can authenticate callers with OAuth."
            action={addButton}
          />
        </ListingTable.Container>
        {dialog}
      </Stack>
    );
  }

  return (
    <ListingTable.Container disablePaper>
      <Box display="flex" justifyContent="flex-end">{addButton}</Box>
      <Stack pt={4}>
        <ListingTable variant="card">
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell width="220px">Name</ListingTable.Cell>
              <ListingTable.Cell width="110px">Type</ListingTable.Cell>
              <ListingTable.Cell>Issuer</ListingTable.Cell>
              <ListingTable.Cell width="140px">Environment</ListingTable.Cell>
              <ListingTable.Cell width="160px" align="right">Gateway</ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {providers.map((provider) => {
              const key = rowKey(provider);
              const isSystem = provider.type === "system";
              return (
                <ListingTable.Row
                  key={key}
                  variant="card"
                  hover
                  onMouseEnter={() => setHoveredKey(key)}
                  onMouseLeave={() => setHoveredKey(null)}
                  onFocus={() => setHoveredKey(key)}
                  onBlur={() => setHoveredKey(null)}
                >
                  <ListingTable.Cell>
                    <Typography variant="body2" fontWeight={500}>
                      {provider.name}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      label={isSystem ? "System" : "Custom"}
                      size="small"
                      color={isSystem ? "default" : "primary"}
                      variant="outlined"
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="caption" color="text.secondary" sx={{ wordBreak: "break-all" }}>
                      {provider.issuer || "—"}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="caption">{provider.environmentName || "—"}</Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    {/* Fixed-height container so swapping the (possibly wrapping)
                        gateway name for the delete icon on hover never changes the
                        row height — the height jump is what caused the flicker. */}
                    <Stack
                      direction="row"
                      alignItems="center"
                      spacing={1}
                      justifyContent="flex-end"
                      sx={{ minHeight: 32 }}
                    >
                      {hoveredKey === key && !isSystem ? (
                        <Tooltip title="Remove identity provider">
                          <IconButton color="error" size="small" onClick={() => openDelete(provider)}>
                            <Trash size={16} />
                          </IconButton>
                        </Tooltip>
                      ) : (
                        <Typography
                          variant="caption"
                          color="text.secondary"
                          noWrap
                          title={provider.gatewayName || undefined}
                        >
                          {provider.gatewayName || "—"}
                        </Typography>
                      )}
                    </Stack>
                  </ListingTable.Cell>
                </ListingTable.Row>
              );
            })}
          </ListingTable.Body>
        </ListingTable>
      </Stack>
      {dialog}
    </ListingTable.Container>
  );
}
