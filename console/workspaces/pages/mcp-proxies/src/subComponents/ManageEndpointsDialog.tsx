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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useUpdateMCPProxy } from "@agent-management-platform/api-client";
import type { Environment, MCPProxy } from "@agent-management-platform/types";
import {
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { useSnackBar } from "@agent-management-platform/views";
import { AddEndpointDialog, type EndpointDraft } from "./AddEndpointDialog";
import { EndpointRow } from "./EndpointRow";
import {
  buildEnvironmentsMap,
  reconstructEndpointsFromEnvironments,
} from "./mcpEndpoints";

interface ManageEndpointsDialogProps {
  open: boolean;
  orgId: string;
  proxy: MCPProxy;
  environments: Environment[];
  onClose: () => void;
}

export function ManageEndpointsDialog({
  open,
  orgId,
  proxy,
  environments,
  onClose,
}: ManageEndpointsDialogProps) {
  const { pushSnackBar } = useSnackBar();
  const updateMCPProxy = useUpdateMCPProxy();

  const [endpoints, setEndpoints] = useState<EndpointDraft[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const endpointIdRef = useRef(1);

  // Seed the working list from the proxy's stored per-environment blocks each time
  // the dialog is opened, grouping blocks that share an upstream into one endpoint.
  useEffect(() => {
    if (!open) return;
    const reconstructed = reconstructEndpointsFromEnvironments(
      proxy.environments ?? {},
    );
    setEndpoints(reconstructed);
    endpointIdRef.current = reconstructed.length + 1;
    setAddOpen(false);
    setEditingId(null);
  }, [open, proxy.environments]);

  const environmentLabels = useMemo(() => {
    const labels = new Map<string, string>();
    environments.forEach((env) => {
      if (env.id) labels.set(env.id, env.displayName || env.name);
    });
    return labels;
  }, [environments]);

  const usedEnvIds = useMemo(() => {
    const used = new Set<string>();
    endpoints.forEach((endpoint) => {
      endpoint.environments.forEach((envId) => used.add(envId));
    });
    return used;
  }, [endpoints]);

  const editingEndpoint = useMemo(
    () => endpoints.find((endpoint) => endpoint.id === editingId),
    [endpoints, editingId],
  );

  // Environments selectable in the endpoint dialog. When editing, the endpoint's own
  // environments stay available; only those claimed by other endpoints are excluded.
  const availableEnvironments = useMemo(() => {
    const ownEnvIds = new Set(editingEndpoint?.environments ?? []);
    return environments.filter(
      (env) => !!env.id && (ownEnvIds.has(env.id) || !usedEnvIds.has(env.id)),
    );
  }, [environments, usedEnvIds, editingEndpoint]);

  const closeEndpointDialog = useCallback(() => {
    setAddOpen(false);
    setEditingId(null);
  }, []);

  const handleAddEndpoint = useCallback((draft: Omit<EndpointDraft, "id">) => {
    const id = String(endpointIdRef.current);
    endpointIdRef.current += 1;
    setEndpoints((current) => [...current, { ...draft, id }]);
    setAddOpen(false);
  }, []);

  const handleSaveEditedEndpoint = useCallback(
    (draft: Omit<EndpointDraft, "id">) => {
      setEndpoints((current) =>
        current.map((endpoint) =>
          endpoint.id === editingId ? { ...draft, id: endpoint.id } : endpoint,
        ),
      );
      setEditingId(null);
    },
    [editingId],
  );

  const handleRemoveEndpoint = useCallback((id: string) => {
    setEndpoints((current) => current.filter((endpoint) => endpoint.id !== id));
  }, []);

  const handleSave = useCallback(async () => {
    try {
      const nextEnvironments = buildEnvironmentsMap(
        endpoints,
        proxy.environments ?? {},
      );
      await updateMCPProxy.mutateAsync({
        params: { orgName: orgId, proxyId: proxy.id },
        body: { ...proxy, environments: nextEnvironments },
      });
      pushSnackBar({
        message: "Endpoints updated successfully.",
        type: "success",
      });
      onClose();
    } catch (err) {
      pushSnackBar({
        message:
          err instanceof Error ? err.message : "Failed to update endpoints.",
        type: "error",
      });
    }
  }, [endpoints, onClose, orgId, proxy, pushSnackBar, updateMCPProxy]);

  const isSaving = updateMCPProxy.isPending;
  // Only consider environments that still exist: usedEnvIds may carry stale IDs
  // reconstructed from the proxy's stored blocks whose environments were since deleted,
  // which would otherwise disable Add Endpoint while live environments remain unclaimed.
  const noEnvironmentsLeft =
    environments.length > 0 &&
    environments.every((env) => !!env.id && usedEnvIds.has(env.id));

  return (
    <>
      <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
        <DialogTitle>Manage Endpoints</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Endpoints map backend MCP servers to environments. Editing an
              endpoint updates the upstream URL, authentication, and
              capabilities for the environments it serves. Environments left
              without an endpoint become unconfigured.
            </Typography>

            {endpoints.length > 0 ? (
              <Stack spacing={1.5}>
                {endpoints.map((endpoint) => (
                  <EndpointRow
                    key={endpoint.id}
                    endpoint={endpoint}
                    environmentLabels={environmentLabels}
                    onEdit={() => setEditingId(endpoint.id)}
                    onRemove={() => handleRemoveEndpoint(endpoint.id)}
                  />
                ))}
              </Stack>
            ) : (
              <Box
                sx={{
                  border: "1px dashed",
                  borderColor: "divider",
                  borderRadius: 1,
                  px: 2,
                  py: 3,
                  textAlign: "center",
                }}
              >
                <Typography variant="body2" color="text.secondary">
                  No endpoints configured yet.
                </Typography>
              </Box>
            )}

            <Box>
              <Tooltip
                title={
                  noEnvironmentsLeft
                    ? "All environments already have an endpoint."
                    : ""
                }
                disableHoverListener={!noEnvironmentsLeft}
              >
                <span>
                  <Button
                    variant="outlined"
                    startIcon={<Plus size={16} />}
                    onClick={() => setAddOpen(true)}
                    disabled={noEnvironmentsLeft || environments.length === 0}
                  >
                    Add Endpoint
                  </Button>
                </span>
              </Tooltip>
            </Box>

            {environments.length > 0 ? (
              <Typography variant="caption" color="text.secondary">
                {usedEnvIds.size} of {environments.length} environments have an
                endpoint.
              </Typography>
            ) : null}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button variant="outlined" onClick={onClose} disabled={isSaving}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={() => void handleSave()}
            disabled={isSaving}
            startIcon={
              isSaving ? (
                <CircularProgress size={16} color="inherit" />
              ) : undefined
            }
          >
            {isSaving ? "Saving" : "Save Changes"}
          </Button>
        </DialogActions>
      </Dialog>

      <AddEndpointDialog
        open={addOpen || editingId !== null}
        orgId={orgId}
        availableEnvironments={availableEnvironments}
        initialDraft={editingEndpoint}
        onClose={closeEndpointDialog}
        onAdd={
          editingId !== null ? handleSaveEditedEndpoint : handleAddEndpoint
        }
      />
    </>
  );
}

export default ManageEndpointsDialog;
