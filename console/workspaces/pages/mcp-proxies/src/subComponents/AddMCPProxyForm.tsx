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

import { useCallback, useState } from "react";
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
  Button,
  CircularProgress,
  Form,
  FormControl,
  FormLabel,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { type EndpointDraft } from "./EndpointFormFields";
import { EndpointsEditorSection } from "./EndpointsEditorSection";
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
  const [addOpen, setAddOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

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

  // Convenience: seed the proxy name/version/context from the first fetched
  // server when the user hasn't typed them yet. They remain fully editable.
  const handleEndpointAdded = useCallback(
    (draft: Omit<EndpointDraft, "id">) => {
      if (!proxyName && draft.serverName) {
        setProxyName(draft.serverName);
        if (!proxyContext) setProxyContext(`/default/${draft.serverName}`);
      }
      if (!proxyVersion && draft.serverVersion) {
        setProxyVersion(draft.serverVersion);
      }
    },
    [proxyContext, proxyName, proxyVersion],
  );

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
      endpoints: (() => {
        const usedHandles = new Set<string>();
        return endpoints.map((endpoint, index) =>
          draftToEndpoint(endpoint, index, undefined, usedHandles),
        );
      })(),
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

  return (
    <Stack spacing={3}>
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

          <EndpointsEditorSection
            orgId={orgId ?? ""}
            environments={environments}
            endpoints={endpoints}
            onEndpointsChange={setEndpoints}
            addOpen={addOpen}
            onAddOpenChange={setAddOpen}
            editingId={editingId}
            onEditingIdChange={setEditingId}
            emptyStateText="No endpoints added yet."
            onEndpointAdded={handleEndpointAdded}
          />
        </Form.Stack>
      </Form.Section>

      {addOpen || editingId !== null ? null : (
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
      )}
    </Stack>
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
