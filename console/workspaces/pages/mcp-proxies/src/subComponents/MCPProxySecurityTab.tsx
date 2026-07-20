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
import {
  useCreateMCPProxyScope,
  useDeleteMCPProxyScope,
  useListMCPProxyScopes,
  useUpdateMCPProxyScope,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import type {
  APIKeyLocation,
  MCPEndpointConfig,
  MCPProxy,
  MCPProxyScopeResponse,
} from "@agent-management-platform/types";
import {
  Alert,
  Autocomplete,
  Button,
  Chip,
  Collapse,
  createFilterOptions,
  FormControl,
  FormLabel,
  Grid,
  ListingTable,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Info } from "@wso2/oxygen-ui-icons-react";
import {
  type AuthenticationType,
  getAuthenticationTypeLabel,
  getCapabilityId,
  isToolBlockedByAcl,
  resolveAuthenticationType,
} from "./mcpEndpoints";

const KEY_LOCATION_OPTIONS: { value: APIKeyLocation; label: string }[] = [
  { value: "header", label: "header" },
  { value: "query", label: "query" },
];

const AUTHENTICATION_TYPE_OPTIONS: AuthenticationType[] = [
  "",
  "apiKey",
  "identity",
];

// A scope option in the per-tool scope Autocomplete. The synthetic "isNew"
// entry stands in for "create this scope (with this row's tool) on save" —
// committing it just adds a pending placeholder to row.scopes; the actual
// create/update/delete calls are reconciled against the server in handleSave.
type ScopeOption = MCPProxyScopeResponse & { isNew?: boolean };

const filterScopeOptions = createFilterOptions<ScopeOption>();

// A local editable row for the identity-security tool-scope-binding table.
// One row per tool discovered on this endpoint — every tool always has a
// row, whether or not it currently has any scopes assigned — so the tool
// itself doubles as the row's key.
type ToolScopeRow = {
  tool: string;
  scopes: MCPProxyScopeResponse[];
};

type ScopeReconciliation = {
  creates: { action: string; tools: string[] }[];
  updates: { action: string; tools: string[] }[];
  deletes: string[];
};

// Diffs the desired (action -> tools) mapping built from the current rows
// against the last-fetched scope list, producing the minimal set of
// create/update/delete operations needed to bring the server in sync.
function computeScopeReconciliation(
  rows: ToolScopeRow[],
  catalogScopes: MCPProxyScopeResponse[],
): ScopeReconciliation {
  const desired = new Map<string, Set<string>>();
  for (const row of rows) {
    for (const scope of row.scopes) {
      const tools = desired.get(scope.action) ?? new Set<string>();
      tools.add(row.tool);
      desired.set(scope.action, tools);
    }
  }
  const original = new Map(catalogScopes.map((s) => [s.action, new Set(s.tools)]));
  const setsEqual = (a: Set<string>, b: Set<string>) =>
    a.size === b.size && [...a].every((v) => b.has(v));

  const creates: { action: string; tools: string[] }[] = [];
  const updates: { action: string; tools: string[] }[] = [];
  const deletes: string[] = [];

  for (const [action, tools] of desired) {
    const originalTools = original.get(action);
    if (!originalTools) {
      creates.push({ action, tools: [...tools] });
    } else if (!setsEqual(originalTools, tools)) {
      updates.push({ action, tools: [...tools] });
    }
  }
  for (const action of original.keys()) {
    if (!desired.has(action)) deletes.push(action);
  }

  return { creates, updates, deletes };
}

export type MCPProxySecurityTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  orgName: string | undefined;
  proxyId: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxySecurityTab({
  config,
  selectedEndpointId,
  orgName,
  proxyId,
  isLoading = false,
  onUpdate,
  isUpdating,
}: MCPProxySecurityTabProps) {
  const [authenticationType, setAuthenticationType] =
    useState<AuthenticationType>("apiKey");
  const [keyValue, setKeyValue] = useState("");
  const [keyIn, setKeyIn] = useState<APIKeyLocation>("header");
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const [fieldErrors, setFieldErrors] = useState<{ keyValue?: string }>({});
  const { addConfirmation } = useConfirmationDialog();

  // Tracks what was last confirmed persisted (seeded from config, refreshed on
  // save) rather than re-deriving "saved" straight from the config prop on
  // every render — config only reflects a save once its background refetch
  // lands, which would otherwise leave authIsDirty true for a beat after a
  // successful save.
  const lastSavedAuthRef = useRef<{
    type: AuthenticationType;
    key: string;
    in: APIKeyLocation;
  }>({ type: "apiKey", key: "", in: "header" });

  const authIsDirty = useMemo(() => {
    if (!config) return false;
    const saved = lastSavedAuthRef.current;
    if (authenticationType !== saved.type) return true;
    if (keyValue.trim() !== saved.key) return true;
    if (keyIn !== saved.in) return true;
    return false;
  }, [config, authenticationType, keyValue, keyIn]);

  useEffect(() => {
    if (!config || !selectedEndpointId) return;
    const nextType = resolveAuthenticationType(config);
    const nextKey =
      config.security?.apiKey?.key ?? (nextType === "apiKey" ? "X-API-Key" : "");
    const nextIn = (config.security?.apiKey?.in as APIKeyLocation) ?? "header";
    setAuthenticationType(nextType);
    setKeyValue(nextKey);
    setKeyIn(nextIn);
    setFieldErrors({});
    lastSavedAuthRef.current = { type: nextType, key: nextKey, in: nextIn };
  }, [config, selectedEndpointId]);

  // --- Agent Identity: per-tool scope-binding (RBAC) state ---

  const toolEntries = useMemo(() => {
    const identifiers: string[] = [];
    for (const raw of config?.capabilities?.tools ?? []) {
      const identifier = getCapabilityId("tool", raw);
      if (identifier) identifiers.push(identifier);
    }
    return identifiers;
  }, [config?.capabilities?.tools]);

  // Computed once per tool list / ACL policy change rather than per row and per
  // dropdown option on every render — isToolBlockedByAcl re-parses the ACL
  // policy's params each call, and every row's Select renders one option per tool.
  const blockedToolIds = useMemo(
    () =>
      new Set(
        toolEntries.filter((identifier) =>
          isToolBlockedByAcl(config, identifier),
        ),
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only reads config.policies
    [toolEntries, config?.policies],
  );

  const { data: scopesData } = useListMCPProxyScopes(
    { orgName: orgName ?? "", proxyId: proxyId ?? "" },
    { enabled: authenticationType === "identity" && !!proxyId },
  );
  const catalogScopes: MCPProxyScopeResponse[] = useMemo(
    () => scopesData?.scopes ?? [],
    [scopesData],
  );
  const createMCPProxyScope = useCreateMCPProxyScope();
  const updateMCPProxyScope = useUpdateMCPProxyScope();
  const deleteMCPProxyScope = useDeleteMCPProxyScope();

  // One row per known tool, pre-populated from the endpoint's discovered
  // tool list so scopes can be assigned directly — no separate "add tool"
  // step. Starts empty and is seeded by the effects below once the tool
  // list and scope catalog are both available.
  const [toolScopeRows, setToolScopeRows] = useState<ToolScopeRow[]>([]);
  const lastSavedToolScopeRowsRef = useRef<ToolScopeRow[]>([]);

  const serializeToolScopeRows = (rows: ToolScopeRow[]) =>
    JSON.stringify(
      rows.map((row) => ({
        tool: row.tool,
        scopes: [...row.scopes.map((s) => s.action)].sort(),
      })),
    );

  const toolScopesDirty = useMemo(() => {
    return (
      serializeToolScopeRows(toolScopeRows) !==
      serializeToolScopeRows(lastSavedToolScopeRowsRef.current)
    );
  }, [toolScopeRows]);

  // Rows are a view built from the discovered tool list, not a separately
  // stored binding: every known tool always has a row, seeded with whichever
  // scopes already reference it (empty if none). Memoized since both reseed
  // effects below would otherwise rebuild this from scratch on every render
  // they fire in, even when neither toolEntries nor catalogScopes changed.
  const derivedToolScopeRows = useMemo<ToolScopeRow[]>(() => {
    const toolToScopes = new Map<string, MCPProxyScopeResponse[]>();
    for (const scope of catalogScopes) {
      for (const tool of scope.tools) {
        const scopesForTool = toolToScopes.get(tool) ?? [];
        scopesForTool.push(scope);
        toolToScopes.set(tool, scopesForTool);
      }
    }
    return toolEntries.map((tool) => ({
      tool,
      scopes: toolToScopes.get(tool) ?? [],
    }));
  }, [toolEntries, catalogScopes]);

  // Switching endpoint tabs discards unsaved row edits, consistent with the
  // auth-fields effect above — even though scopes are shared across every
  // endpoint of the proxy, this tab's Save/Discard state is still per endpoint.
  // Reads derivedToolScopeRows without depending on it — the effect below
  // owns reseeding on scope-list changes, so this one only reacts to the tab switch.
  useEffect(() => {
    if (!selectedEndpointId) return;
    setToolScopeRows(derivedToolScopeRows);
    lastSavedToolScopeRowsRef.current = derivedToolScopeRows;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- handled by the effect below
  }, [selectedEndpointId]);

  // Reseed when the scope list refetches (e.g. right after Save invalidates
  // the query), but only while there are no unsaved edits — otherwise this
  // would clobber in-progress changes on a stray background refetch. Doesn't
  // depend on selectedEndpointId — the effect above already handles tab
  // switches, and scopes are proxy-level so switching tabs alone never
  // changes catalogScopes.
  useEffect(() => {
    if (!selectedEndpointId || toolScopesDirty) return;
    setToolScopeRows(derivedToolScopeRows);
    lastSavedToolScopeRowsRef.current = derivedToolScopeRows;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- guard, not a trigger dep
  }, [catalogScopes]);

  const setRowScopes = useCallback((tool: string, scopes: MCPProxyScopeResponse[]) => {
    setToolScopeRows((prev) =>
      prev.map((row) => (row.tool === tool ? { ...row, scopes } : row)),
    );
  }, []);

  // Selecting the synthetic "+ Add Scope" option just commits a pending
  // placeholder to the row — it isn't created on the server until Save,
  // when the final set of tools across every row referencing this action is
  // known (see handleSave's reconciliation against the fetched scope list).
  const handleToolScopeRowScopesChange = useCallback(
    (tool: string, options: ScopeOption[]) => {
      setRowScopes(tool, options);
    },
    [setRowScopes],
  );

  // Confirm before switching methods — it breaks agents already configured
  // to use this proxy. Reads the saved type from `config`, not
  // lastSavedAuthRef, which defaults to "apiKey" until the sync effect above
  // runs and would otherwise warn about a method nobody actually configured.
  const handleAuthTypeChange = useCallback(
    (nextType: AuthenticationType) => {
      const savedType = resolveAuthenticationType(config);
      if (savedType && nextType !== savedType) {
        addConfirmation({
          title: "Switch authentication method?",
          description: `This proxy is currently secured with ${getAuthenticationTypeLabel(savedType)}. Switching to ${getAuthenticationTypeLabel(nextType)} will break any agent already configured to use it, until their tool configuration is updated to match.`,
          confirmButtonText: "Switch Method",
          confirmButtonColor: "error",
          onConfirm: () => setAuthenticationType(nextType),
        });
        return;
      }
      setAuthenticationType(nextType);
    },
    [addConfirmation, config],
  );

  const isDirty = authIsDirty || toolScopesDirty;

  const handleDiscard = useCallback(() => {
    if (!config) return;
    const nextType = resolveAuthenticationType(config);
    setAuthenticationType(nextType);
    setKeyValue(
      config.security?.apiKey?.key ??
      (nextType === "apiKey" ? "X-API-Key" : ""),
    );
    setKeyIn((config.security?.apiKey?.in as APIKeyLocation) ?? "header");
    setFieldErrors({});
    setStatus(null);

    setToolScopeRows(lastSavedToolScopeRowsRef.current);
  }, [config]);

  const handleSave = useCallback(async () => {
    if (!config) return;

    if (authenticationType === "apiKey" && keyValue.trim().length === 0) {
      const message = "API Key is required when using API Key authentication";
      setFieldErrors({ keyValue: message });
      setStatus({ message, severity: "error" });
      return;
    }
    setFieldErrors({});

    try {
      await onUpdate({
        security: {
          enabled: config.security?.enabled ?? true,
          apiKey: {
            enabled: authenticationType === "apiKey",
            key: authenticationType === "apiKey" ? keyValue.trim() : "",
            in: keyIn,
          },
          identity: {
            enabled: authenticationType === "identity",
          },
        },
      });

      // Scopes belong to the proxy, not this endpoint's auth mode, and are
      // saved via their own REST calls rather than bundled into the security
      // payload above.
      if (authenticationType === "identity" && toolScopesDirty && orgName && proxyId) {
        const { creates, updates, deletes } = computeScopeReconciliation(
          toolScopeRows,
          catalogScopes,
        );

        await Promise.all([
          ...creates.map((c) =>
            createMCPProxyScope.mutateAsync({
              params: { orgName, proxyId },
              body: { action: c.action, tools: c.tools },
            }),
          ),
          ...updates.map((u) =>
            updateMCPProxyScope.mutateAsync({
              params: { orgName, proxyId, scopeAction: u.action },
              body: { tools: u.tools },
            }),
          ),
          ...deletes.map((action) =>
            deleteMCPProxyScope.mutateAsync({ orgName, proxyId, scopeAction: action }),
          ),
        ]);

        lastSavedToolScopeRowsRef.current = toolScopeRows;
      }

      lastSavedAuthRef.current = {
        type: authenticationType,
        key: authenticationType === "apiKey" ? keyValue.trim() : "",
        in: keyIn,
      };
      setStatus({
        message: "Updated security settings.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to update security.",
        severity: "error",
      });
    }
  }, [
    config,
    authenticationType,
    keyValue,
    keyIn,
    toolScopeRows,
    toolScopesDirty,
    catalogScopes,
    orgName,
    proxyId,
    onUpdate,
    createMCPProxyScope,
    updateMCPProxyScope,
    deleteMCPProxyScope,
  ]);

  const isDisabled = isLoading || !config;
  const noToolsForRbac = !isLoading && config && toolEntries.length === 0;

  if (isLoading) {
    return (
      <Stack spacing={2}>
        <Typography variant="h6">Authentication</Typography>
        <Stack spacing={2}>
          {[1, 2, 3].map((i) => (
            <Stack key={i} spacing={0.5}>
              <Skeleton variant="text" width={120} height={16} />
              <Skeleton variant="rounded" height={40} />
            </Stack>
          ))}
        </Stack>
      </Stack>
    );
  }

  if (!config) {
    return null;
  }

  return (
    <Stack spacing={2}>
      <Typography variant="h6">Authentication</Typography>

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 5 }}>
          <FormControl fullWidth disabled={isDisabled}>
            <FormLabel>Method</FormLabel>
            <Select
              size="small"
              displayEmpty
              value={authenticationType || ""}
              onChange={(e) =>
                handleAuthTypeChange((e.target.value as AuthenticationType) || "")
              }
            >
              {AUTHENTICATION_TYPE_OPTIONS.map((type) => (
                <MenuItem key={type || "none"} value={type}>
                  {getAuthenticationTypeLabel(type)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
      </Grid>

      {authenticationType === "identity" && (
        <Stack spacing={2}>
          <Stack spacing={0.5}>
            <Typography variant="h6">Authorization</Typography>
            <Typography variant="body2" color="text.secondary">
              Restrict access to individual tools by assigning catalog
              scopes. Callers need a token that includes every scope
              required by a tool to invoke it.
            </Typography>
          </Stack>

          {noToolsForRbac ? (
            <Stack
              alignItems="center"
              justifyContent="center"
              spacing={1}
              sx={{ minHeight: 200, textAlign: "center" }}
            >
              <Typography variant="subtitle1" fontWeight={600}>
                No Tools Available
              </Typography>
              <Typography variant="body2" color="text.secondary">
                This MCP proxy has no tools. Scope bindings require at least one
                tool.
              </Typography>
            </Stack>
          ) : (
            <ListingTable.Container>
              <ListingTable density="compact">
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell width="30%">Tool</ListingTable.Cell>
                    <ListingTable.Cell>Scopes</ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {toolScopeRows.map((row) => (
                    <ListingTable.Row key={row.tool}>
                      <ListingTable.Cell>
                        <Stack direction="row" alignItems="center" spacing={1}>
                          <Typography
                            component="span"
                            variant="body2"
                            sx={{ fontFamily: "monospace" }}
                            noWrap
                          >
                            {row.tool}
                          </Typography>
                          {blockedToolIds.has(row.tool) && (
                            <Tooltip title="Blocked by Manage Tools">
                              <Stack color="warning.main" direction="row" alignItems="center">
                                <Info size={14} />
                              </Stack>
                            </Tooltip>
                          )}
                        </Stack>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Autocomplete
                          multiple
                          size="small"
                          disableCloseOnSelect
                          options={catalogScopes}
                          value={row.scopes}
                          onChange={(_e, value) =>
                            handleToolScopeRowScopesChange(
                              row.tool,
                              value as ScopeOption[],
                            )
                          }
                          filterOptions={(options, params) => {
                            const filtered = filterScopeOptions(
                              options as ScopeOption[],
                              params,
                            );
                            const inputValue = params.inputValue.trim();
                            const exists = options.some(
                              (option) => option.action === inputValue,
                            );
                            if (inputValue.length > 0 && !exists) {
                              filtered.push({
                                action: inputValue,
                                scope: inputValue,
                                tools: [],
                                isNew: true,
                              });
                            }
                            return filtered;
                          }}
                          getOptionLabel={(option) =>
                            (option as ScopeOption).action
                          }
                          isOptionEqualToValue={(option, value) =>
                            (option as ScopeOption).action ===
                            (value as ScopeOption).action
                          }
                          renderOption={(props, option) => {
                            const scopeOption = option as ScopeOption;
                            return (
                              <li {...props} key={scopeOption.action}>
                                {scopeOption.isNew
                                  ? `+ Add Scope "${scopeOption.action}"`
                                  : scopeOption.action}
                              </li>
                            );
                          }}
                          renderTags={(value, getTagProps) =>
                            value.map((option, index) => (
                              <Chip
                                {...getTagProps({ index })}
                                key={option.action}
                                label={option.action}
                                size="small"
                              />
                            ))
                          }
                          renderInput={(params) => (
                            <TextField {...params} placeholder="Add scopes..." />
                          )}
                          noOptionsText="No scopes in the catalog"
                          sx={{ minWidth: 280 }}
                        />
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          )}
        </Stack>
      )}

      {authenticationType === "apiKey" && (
        <Grid container spacing={3}>
          <Grid size={{ xs: 12, md: 5 }}>
            <FormControl fullWidth disabled={isDisabled}>
              <FormLabel>Key Location</FormLabel>
              <Select
                size="small"
                value={keyIn}
                onChange={(e) => setKeyIn(e.target.value as APIKeyLocation)}
              >
                {KEY_LOCATION_OPTIONS.map((opt) => (
                  <MenuItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid size={{ xs: 12, md: 5 }}>
            <FormControl
              fullWidth
              disabled={isDisabled}
              error={!!fieldErrors.keyValue}
            >
              <FormLabel>
                {keyIn === "query" ? "Query Param Key" : "Header Key"}
              </FormLabel>
              <TextField
                size="small"
                value={keyValue}
                onChange={(e) => {
                  setKeyValue(e.target.value);
                  if (fieldErrors.keyValue) setFieldErrors({});
                }}
                error={!!fieldErrors.keyValue}
                helperText={fieldErrors.keyValue}
                sx={{
                  "& .MuiInputBase-input": {
                    fontFamily: "monospace",
                  },
                }}
              />
            </FormControl>
          </Grid>
        </Grid>
      )}

      <Stack spacing={1.5} width="100%">
        <Collapse in={!!status && !isDirty} timeout={300}>
          {status && (
            <Alert
              severity={status.severity}
              onClose={() => setStatus(null)}
              sx={{ width: "100%", maxWidth: 480 }}
            >
              {status.message}
            </Alert>
          )}
        </Collapse>
        <Stack direction="row" spacing={1.5} justifyContent="flex-end">
          <Button
            variant="outlined"
            onClick={handleDiscard}
            disabled={!isDirty || isUpdating}
          >
            Discard
          </Button>
          <Button
            variant="contained"
            onClick={() => void handleSave()}
            disabled={isUpdating || !isDirty}
          >
            {isUpdating ? "Saving..." : "Save"}
          </Button>
        </Stack>
      </Stack>
    </Stack>
  );
}
