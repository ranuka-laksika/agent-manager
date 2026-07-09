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

import { useCallback, useMemo, useRef, useState } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useCreateMCPProxy,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type MCPProxy,
} from "@agent-management-platform/types";
import {
  Box,
  Button,
  CircularProgress,
  Form,
  FormControl,
  FormLabel,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { AddEndpointDialog, type EndpointDraft } from "./AddEndpointDialog";
import { EndpointRow } from "./EndpointRow";
import { draftToEndpoint } from "./mcpEndpoints";
import { MCP_SPEC_VERSION } from "../constants";

interface AddMCPProxyFormProps {
  onCancel: () => void;
}

export function AddMCPProxyForm({ onCancel }: AddMCPProxyFormProps) {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const createMCPProxy = useCreateMCPProxy();
  const { data: environments = [] } = useListEnvironments({
    orgName: orgId ?? "",
  });

  const [proxyName, setProxyName] = useState("");
  const [proxyVersion, setProxyVersion] = useState("");
  const [proxyDescription, setProxyDescription] = useState("");
  const [proxyContext, setProxyContext] = useState("");
  const [endpoints, setEndpoints] = useState<EndpointDraft[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const endpointIdRef = useRef(1);

  // Map of environment UUID -> display label, for rendering endpoint chips.
  const environmentLabels = useMemo(() => {
    const labels = new Map<string, string>();
    environments.forEach((env) => {
      if (env.id) labels.set(env.id, env.displayName || env.name);
    });
    return labels;
  }, [environments]);

  // Environment UUIDs already claimed by an endpoint (one env maps to one endpoint).
  const usedEnvIds = useMemo(() => {
    const used = new Set<string>();
    endpoints.forEach((endpoint) => {
      endpoint.environments.forEach((envId) => used.add(envId));
    });
    return used;
  }, [endpoints]);

  const availableEnvironments = useMemo(
    () => environments.filter((env) => !!env.id && !usedEnvIds.has(env.id)),
    [environments, usedEnvIds],
  );

  const isCreating = createMCPProxy.isPending;

  const handleNameChange = useCallback(
    (value: string) => {
      const previousContext = proxyName ? `/default/${proxyName}` : "";
      setProxyName(value);
      if (!proxyContext || proxyContext === previousContext) {
        setProxyContext(value ? `/default/${value}` : "");
      }
    },
    [proxyContext, proxyName],
  );

  const handleAddEndpoint = useCallback(
    (draft: Omit<EndpointDraft, "id">) => {
      const id = String(endpointIdRef.current);
      endpointIdRef.current += 1;
      setEndpoints((current) => [...current, { ...draft, id }]);

      // Convenience: seed the proxy name/version/context from the first fetched
      // server when the user hasn't typed them yet. They remain fully editable.
      if (!proxyName && draft.serverName) {
        setProxyName(draft.serverName);
        if (!proxyContext) setProxyContext(`/default/${draft.serverName}`);
      }
      if (!proxyVersion && draft.serverVersion) {
        setProxyVersion(draft.serverVersion);
      }
      setDialogOpen(false);
    },
    [proxyContext, proxyName, proxyVersion],
  );

  const handleRemoveEndpoint = useCallback((id: string) => {
    setEndpoints((current) => current.filter((endpoint) => endpoint.id !== id));
  }, []);

  const handleCreate = useCallback(async () => {
    if (!orgId || endpoints.length === 0) return;

    const name = proxyName.trim();

    // Each endpoint carries its own upstream, fetched capabilities and default security,
    // and is bound to one or more environments. The org-level proxy is a grouping and
    // deploys nothing itself.
    const body: MCPProxy = {
      id: toHandle(name),
      name,
      version: proxyVersion.trim(),
      description: proxyDescription.trim() || undefined,
      context: proxyContext.trim() || undefined,
      mcpSpecVersion: MCP_SPEC_VERSION,
      endpoints: endpoints.map((endpoint, index) =>
        draftToEndpoint(endpoint, index),
      ),
    };

    await createMCPProxy.mutateAsync({ params: { orgName: orgId }, body });
    navigate(
      generatePath(absoluteRouteMap.children.org.children.mcpProxies.path, {
        orgId,
      }),
    );
  }, [
    createMCPProxy,
    endpoints,
    navigate,
    orgId,
    proxyContext,
    proxyDescription,
    proxyName,
    proxyVersion,
  ]);

  const canCreate =
    Boolean(proxyName.trim()) &&
    Boolean(proxyVersion.trim()) &&
    endpoints.length > 0 &&
    !isCreating;

  const noEnvironmentsLeft =
    environments.length > 0 && availableEnvironments.length === 0;

  return (
    <>
      <Stack spacing={3} sx={{ maxWidth: 920 }}>
        <Form.Section>
          <Form.Stack spacing={2}>
            <Form.Stack
              direction={{ xs: "column", md: "row" }}
              spacing={2}
              useFlexGap
            >
              <FormControl sx={{ flex: 1 }}>
                <FormLabel required>Name</FormLabel>
                <TextField
                  fullWidth
                  value={proxyName}
                  onChange={(event) => handleNameChange(event.target.value)}
                />
              </FormControl>
              <FormControl sx={{ width: { xs: "100%", md: 300 } }}>
                <FormLabel required>Version</FormLabel>
                <TextField
                  fullWidth
                  value={proxyVersion}
                  onChange={(event) => setProxyVersion(event.target.value)}
                />
              </FormControl>
            </Form.Stack>

            <FormControl fullWidth>
              <FormLabel>Description</FormLabel>
              <TextField
                fullWidth
                multiline
                minRows={3}
                value={proxyDescription}
                onChange={(event) => setProxyDescription(event.target.value)}
                placeholder="Primary MCP Proxy"
              />
            </FormControl>

            <FormControl fullWidth>
              <FormLabel>Context</FormLabel>
              <TextField
                fullWidth
                value={proxyContext}
                onChange={(event) => setProxyContext(event.target.value)}
              />
            </FormControl>
          </Form.Stack>
        </Form.Section>

        <Form.Section>
          <Form.Header>Endpoints</Form.Header>
          <Form.Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              Add a backend endpoint and assign it to one or more environments.
              Environments without an endpoint are simply left unconfigured.
            </Typography>

            {endpoints.length > 0 ? (
              <Stack spacing={1.5}>
                {endpoints.map((endpoint) => (
                  <EndpointRow
                    key={endpoint.id}
                    endpoint={endpoint}
                    environmentLabels={environmentLabels}
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
                  No endpoints added yet.
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
                    onClick={() => setDialogOpen(true)}
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
          </Form.Stack>
        </Form.Section>

        <Stack direction="row" spacing={1}>
          <Button variant="outlined" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            variant="contained"
            disabled={!canCreate}
            onClick={handleCreate}
            startIcon={
              isCreating ? (
                <CircularProgress size={16} color="inherit" />
              ) : undefined
            }
          >
            {isCreating ? "Creating" : "Create"}
          </Button>
        </Stack>
      </Stack>

      <AddEndpointDialog
        open={dialogOpen}
        orgId={orgId ?? ""}
        availableEnvironments={availableEnvironments}
        onClose={() => setDialogOpen(false)}
        onAdd={handleAddEndpoint}
      />
    </>
  );
}

function toHandle(value: string): string {
  const handle = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return handle || "mcp-proxy";
}
