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

import { type MouseEvent, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  IconButton,
  ListingTable,
  Skeleton,
  Snackbar,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, ChevronRight, Copy, KeyRound } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useListThunderInstances } from "@agent-management-platform/api-client";
import { absoluteRouteMap, type ThunderInstanceResponse } from "@agent-management-platform/types";

export function ThunderInstancesTable() {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const [snackbarOpen, setSnackbarOpen] = useState(false);

  const { data, isLoading, error } = useListThunderInstances({ orgName: orgId });
  const instances = data?.thunderInstances ?? [];

  const handleCopy = (e: MouseEvent<HTMLButtonElement>, value: string) => {
    e.stopPropagation();
    navigator.clipboard.writeText(value).then(() => setSnackbarOpen(true)).catch(() => {});
  };

  const handleRowClick = (instance: ThunderInstanceResponse) => {
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.thunderInstances.children.view.path,
        { orgId: orgId ?? "", envName: instance.envName },
      ),
    );
  };

  if (isLoading) {
    return (
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
  }

  if (error) {
    return (
      <Alert severity="error" icon={<AlertTriangle size={18} />}>
        Failed to load identity providers. Please try again.
      </Alert>
    );
  }

  if (instances.length === 0) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<KeyRound size={64} />}
          title="No identity providers"
          description="Add an environment first. Each environment automatically gets a Thunder OAuth2 identity provider."
        />
      </ListingTable.Container>
    );
  }

  return (
    <>
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
              onClick={() => handleRowClick(instance)}
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
                    {(instance.displayName || instance.envName).charAt(0).toUpperCase()}
                  </Avatar>
                  <Box>
                    <Typography variant="body2" fontWeight={500}>
                      {instance.displayName || instance.envName}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
                      {instance.envName}
                    </Typography>
                  </Box>
                </Stack>
              </ListingTable.Cell>

              <ListingTable.Cell>
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
                    <IconButton size="small" onClick={(e) => handleCopy(e, instance.tokenUrl)}>
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

      <Snackbar
        open={snackbarOpen}
        autoHideDuration={3000}
        onClose={() => setSnackbarOpen(false)}
        message="Token endpoint copied to clipboard"
      />
    </>
  );
}
