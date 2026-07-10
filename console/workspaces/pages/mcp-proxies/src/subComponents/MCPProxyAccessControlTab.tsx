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
  AccessControlPanel,
  type AccessControlItem,
  type AccessControlMode,
  type AccessControlStatus,
} from "@agent-management-platform/shared-component";
import {
  useCreateScope,
  useListScopes,
  useMCPPoliciesCatalog,
} from "@agent-management-platform/api-client";
import type {
  MCPEndpointConfig,
  MCPProxy,
  MCPProxyPolicy,
  MCPToolScopeBinding,
  ScopeResponse,
} from "@agent-management-platform/types";
import {
  Alert,
  Autocomplete,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  createFilterOptions,
  IconButton,
  ListingTable,
  MenuItem,
  Select,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { HelpCircle, Plus, Trash, Wrench } from "@wso2/oxygen-ui-icons-react";
import { ACL_POLICY_NAME } from "../constants";
import { isIdentitySecurityEnabled } from "./mcpEndpoints";

type CapabilityKind = "tool" | "resource" | "prompt";

// A scope option in the per-tool scope Autocomplete. The synthetic "isNew"
// entry stands in for "create this scope and add it" — it never reaches
// row.scopes, so `id` only needs to be present to satisfy ScopeResponse.
type ScopeOption = ScopeResponse & { isNew?: boolean };

const NEW_SCOPE_OPTION_ID = "__new_scope__";
const filterScopeOptions = createFilterOptions<ScopeOption>();

// Under Agent Identity security, access can be wide open ("allowAll", the
// default — any caller with a valid token can invoke any tool) or gated
// per-tool by catalog scopes ("rbac"). There's no dedicated field for this on
// MCPEndpointConfig — it's derived from whether toolScopeBindings is
// empty, the same way the Security tab derives its auth method from
// security.apiKey/security.identity rather than a separate field.
type IdentityAccessMode = "allowAll" | "rbac";

const ALLOW_ALL_TOOLTIP =
  "Allow all lets any caller with a valid Agent Identity token invoke every tool.";
const RBAC_TOOLTIP =
  "Use RBAC gates each tool by the catalog scopes assigned to it — callers must present a token carrying every required scope.";

// A local editable row for the identity-security tool-scope-binding table.
// Keyed by a local id (not the tool name) since the same tool can appear in
// more than one binding.
type ToolScopeRow = {
  id: number;
  tool: string;
  scopes: ScopeResponse[];
};

const KIND_CHIP_LABEL: Record<CapabilityKind, string> = {
  tool: "Tool",
  resource: "Resource",
  prompt: "Prompt",
};

function getCapabilityIdentifier(
  kind: CapabilityKind,
  raw: Record<string, unknown> | undefined,
): string | null {
  if (!raw) return null;
  const value = kind === "resource" ? raw.uri ?? raw.name : raw.name ?? raw.uri;
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed.length ? trimmed : null;
}

function getCapabilityDescription(
  raw: Record<string, unknown> | undefined,
): string | undefined {
  if (!raw) return undefined;
  const value = raw.description ?? raw.title;
  return typeof value === "string" && value.trim() ? value : undefined;
}

function makeItemKey(kind: CapabilityKind, identifier: string): string {
  return `${kind}::${identifier}`;
}

type ParsedAcl = {
  mode: AccessControlMode;
  exceptionKeys: string[];
};

function parseExistingAclPolicy(
  policy: MCPProxyPolicy | undefined,
): ParsedAcl {
  if (!policy?.params) {
    return { mode: "allow", exceptionKeys: [] };
  }
  const params = policy.params as Record<string, unknown>;
  const sections: CapabilityKind[] = ["tool", "resource", "prompt"];
  const sectionKey: Record<CapabilityKind, string> = {
    tool: "tools",
    resource: "resources",
    prompt: "prompts",
  };
  let resolvedMode: AccessControlMode | null = null;
  const exceptionKeys: string[] = [];
  for (const kind of sections) {
    const section = params[sectionKey[kind]] as
      | Record<string, unknown>
      | undefined;
    if (!section) continue;
    const rawMode = String(section.mode ?? "").toLowerCase();
    if (resolvedMode === null && (rawMode === "allow" || rawMode === "deny")) {
      resolvedMode = rawMode;
    }
    const exceptions = section.exceptions;
    if (Array.isArray(exceptions)) {
      for (const entry of exceptions) {
        if (typeof entry === "string" && entry.trim()) {
          exceptionKeys.push(makeItemKey(kind, entry.trim()));
        }
      }
    }
  }
  return {
    mode: resolvedMode ?? "allow",
    exceptionKeys,
  };
}

function buildAclPolicyParams(
  mode: AccessControlMode,
  exceptionKeys: string[],
  hasCapabilities: Record<CapabilityKind, boolean>,
): Record<string, unknown> {
  const sectionKey: Record<CapabilityKind, string> = {
    tool: "tools",
    resource: "resources",
    prompt: "prompts",
  };
  const exceptionsByKind: Record<CapabilityKind, string[]> = {
    tool: [],
    resource: [],
    prompt: [],
  };
  for (const key of exceptionKeys) {
    const separator = key.indexOf("::");
    if (separator < 0) continue;
    const kind = key.slice(0, separator) as CapabilityKind;
    const identifier = key.slice(separator + 2);
    if (!identifier) continue;
    if (kind in exceptionsByKind) {
      exceptionsByKind[kind].push(identifier);
    }
  }
  const params: Record<string, unknown> = {};
  (["tool", "resource", "prompt"] as CapabilityKind[]).forEach((kind) => {
    if (!hasCapabilities[kind]) return;
    params[sectionKey[kind]] = {
      mode,
      exceptions: exceptionsByKind[kind],
    };
  });
  return params;
}

export type MCPProxyAccessControlTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  orgName: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxyAccessControlTab({
  config,
  selectedEndpointId,
  orgName,
  isLoading = false,
  onUpdate,
  isUpdating,
}: MCPProxyAccessControlTabProps) {
  const lastSavedRef = useRef<{
    mode: AccessControlMode;
    exceptionKeys: string[];
  } | null>(null);

  const [mode, setMode] = useState<AccessControlMode>("allow");
  const [exceptionKeys, setExceptionKeys] = useState<string[]>([]);
  const [status, setStatus] = useState<AccessControlStatus | null>(null);

  const { data: catalog, isLoading: isCatalogLoading } = useMCPPoliciesCatalog(
    orgName,
  );
  const availableAclPolicy = useMemo(
    () => catalog?.data?.find((p) => p.name === ACL_POLICY_NAME),
    [catalog],
  );

  const items = useMemo<AccessControlItem[]>(() => {
    const capabilities = config?.capabilities;
    if (!capabilities) return [];
    const result: AccessControlItem[] = [];
    const kinds: Array<{ kind: CapabilityKind; entries?: Record<string, unknown>[] }> = [
      { kind: "tool", entries: capabilities.tools },
      { kind: "resource", entries: capabilities.resources },
      { kind: "prompt", entries: capabilities.prompts },
    ];
    for (const { kind, entries } of kinds) {
      (entries ?? []).forEach((raw) => {
        const identifier = getCapabilityIdentifier(kind, raw);
        if (!identifier) return;
        result.push({
          key: makeItemKey(kind, identifier),
          method: KIND_CHIP_LABEL[kind],
          path: identifier,
          summary: getCapabilityDescription(raw),
        });
      });
    }
    return result;
  }, [config?.capabilities]);

  const capabilitiesPresent = useMemo<Record<CapabilityKind, boolean>>(
    () => ({
      tool: Boolean(config?.capabilities?.tools?.length),
      resource: Boolean(config?.capabilities?.resources?.length),
      prompt: Boolean(config?.capabilities?.prompts?.length),
    }),
    [
      config?.capabilities?.tools,
      config?.capabilities?.resources,
      config?.capabilities?.prompts,
    ],
  );

  const hasAnyCapability =
    capabilitiesPresent.tool ||
    capabilitiesPresent.resource ||
    capabilitiesPresent.prompt;

  // Agent Identity security replaces the allow/deny ACL policy below with a
  // per-tool scope-binding table: callers must present a token carrying every
  // scope assigned to the tool they're invoking.
  const isIdentitySecurity = isIdentitySecurityEnabled(config);

  const toolEntries = useMemo(() => {
    const entries: { identifier: string; description?: string }[] = [];
    for (const raw of config?.capabilities?.tools ?? []) {
      const identifier = getCapabilityIdentifier("tool", raw);
      if (!identifier) continue;
      entries.push({ identifier, description: getCapabilityDescription(raw) });
    }
    return entries;
  }, [config?.capabilities?.tools]);

  const { data: scopesData } = useListScopes({ orgName });
  const catalogScopes: ScopeResponse[] = useMemo(
    () => scopesData?.scopes ?? [],
    [scopesData],
  );
  const createScope = useCreateScope();

  // Read via a ref inside the seed effect below rather than depending on
  // catalogScopes directly — the catalog refetches whenever a scope is
  // created inline (see handleToolScopeRowScopesChange), and re-seeding on
  // every refetch would blow away rows/edits the user hasn't saved yet.
  const catalogScopesRef = useRef<ScopeResponse[]>([]);
  useEffect(() => {
    catalogScopesRef.current = catalogScopes;
  }, [catalogScopes]);

  // One row per binding, not one row per tool: the same tool can be bound
  // more than once, so rows are keyed by a local id rather than the tool
  // name. Starts empty — rows only come from saved bindings or "Add Tool",
  // never auto-populated from the environment's discovered tools.
  const [toolScopeRows, setToolScopeRows] = useState<ToolScopeRow[]>([]);
  const lastSavedToolScopeRowsRef = useRef<ToolScopeRow[]>([]);
  const nextRowIdRef = useRef(0);
  const [identityMode, setIdentityMode] = useState<IdentityAccessMode>("allowAll");
  const lastSavedIdentityModeRef = useRef<IdentityAccessMode>("allowAll");
  const [toolScopesError, setToolScopesError] = useState<string | undefined>();
  // Tracks which row is currently creating a new catalog scope inline, so its
  // Autocomplete alone shows a loading spinner while the create request is in flight.
  const [creatingScopeRowId, setCreatingScopeRowId] = useState<number | null>(
    null,
  );

  useEffect(() => {
    if (!selectedEndpointId) return;
    // Bindings for scopes no longer in the catalog still display (by name)
    // rather than silently dropping, so a removed/renamed scope stays visible
    // until the binding itself is edited.
    const rows: ToolScopeRow[] = (config?.toolScopeBindings ?? []).map(
      (binding) => ({
        id: nextRowIdRef.current++,
        tool: binding.tool,
        scopes: binding.scopes.map(
          (name) =>
            catalogScopesRef.current.find((s) => s.name === name) ?? {
              id: name,
              name,
            },
        ),
      }),
    );
    const derivedMode: IdentityAccessMode = rows.length > 0 ? "rbac" : "allowAll";
    setToolScopeRows(rows);
    lastSavedToolScopeRowsRef.current = rows;
    setIdentityMode(derivedMode);
    lastSavedIdentityModeRef.current = derivedMode;
    setToolScopesError(undefined);
  }, [config, selectedEndpointId]);

  const serializeToolScopeRows = (rows: ToolScopeRow[]) =>
    JSON.stringify(
      rows.map((row) => ({
        tool: row.tool,
        scopes: [...row.scopes.map((s) => s.name)].sort(),
      })),
    );

  const toolScopesDirty = useMemo(() => {
    if (identityMode !== lastSavedIdentityModeRef.current) return true;
    if (identityMode === "allowAll") return false;
    return (
      serializeToolScopeRows(toolScopeRows) !==
      serializeToolScopeRows(lastSavedToolScopeRowsRef.current)
    );
  }, [identityMode, toolScopeRows]);

  const handleAddToolScopeRow = useCallback(() => {
    setToolScopeRows((prev) => [
      ...prev,
      { id: nextRowIdRef.current++, tool: "", scopes: [] },
    ]);
  }, []);

  const handleRemoveToolScopeRow = useCallback((rowId: number) => {
    setToolScopeRows((prev) => prev.filter((row) => row.id !== rowId));
  }, []);

  const handleToolScopeRowToolChange = useCallback(
    (rowId: number, tool: string) => {
      setToolScopeRows((prev) =>
        prev.map((row) => (row.id === rowId ? { ...row, tool } : row)),
      );
    },
    [],
  );

  const setRowScopes = useCallback((rowId: number, scopes: ScopeResponse[]) => {
    setToolScopeRows((prev) =>
      prev.map((row) => (row.id === rowId ? { ...row, scopes } : row)),
    );
  }, []);

  // Selecting the synthetic "+ Add Scope" option creates it in the catalog
  // first, then adds the real scope to the row once creation succeeds — the
  // placeholder never lands in row.scopes, so a failed create just leaves the
  // row as it was (the error itself surfaces via useCreateScope's snackbar).
  const handleToolScopeRowScopesChange = useCallback(
    (rowId: number, options: ScopeOption[]) => {
      const newOption = options.find((option) => option.isNew);
      if (!newOption) {
        setRowScopes(rowId, options);
        return;
      }
      const committed = options.filter((option) => option !== newOption);
      setRowScopes(rowId, committed);
      setCreatingScopeRowId(rowId);
      void createScope
        .mutateAsync({ params: { orgName }, body: { name: newOption.name } })
        .then((created) => {
          setToolScopeRows((prev) =>
            prev.map((row) =>
              row.id === rowId
                ? { ...row, scopes: [...row.scopes, created] }
                : row,
            ),
          );
        })
        .catch(() => {
          // Error already shown via useCreateScope's snackbar; nothing to roll back
          // since the placeholder was never committed to row.scopes.
        })
        .finally(() => {
          setCreatingScopeRowId((current) => (current === rowId ? null : current));
        });
    },
    [orgName, createScope, setRowScopes],
  );

  const handleSaveToolScopes = useCallback(async () => {
    const nextBindings: MCPToolScopeBinding[] =
      identityMode === "allowAll"
        ? []
        : toolScopeRows.map((row) => ({
            tool: row.tool,
            scopes: row.scopes.map((s) => s.name),
          }));

    if (
      identityMode === "rbac" &&
      toolScopeRows.some((row) => !row.tool || row.scopes.length === 0)
    ) {
      setToolScopesError(
        "Every row needs a tool and at least one scope before saving.",
      );
      return;
    }
    setToolScopesError(undefined);

    try {
      await onUpdate({ toolScopeBindings: nextBindings });
      lastSavedToolScopeRowsRef.current = toolScopeRows;
      lastSavedIdentityModeRef.current = identityMode;
    } catch {
      // Save success/failure is already reported via the mutation's own
      // snackbar (see useUpdateMCPProxy) — nothing more to show here.
    }
  }, [identityMode, toolScopeRows, onUpdate]);

  const handleDiscardToolScopes = useCallback(() => {
    setToolScopeRows(lastSavedToolScopeRowsRef.current);
    setIdentityMode(lastSavedIdentityModeRef.current);
    setToolScopesError(undefined);
  }, []);

  useEffect(() => {
    if (!selectedEndpointId) return;
    const existing = config?.policies?.find((p) => p.name === ACL_POLICY_NAME);
    const parsed = parseExistingAclPolicy(existing);
    setMode(parsed.mode);
    setExceptionKeys(parsed.exceptionKeys);
    lastSavedRef.current = parsed;
  }, [config, selectedEndpointId]);

  const isDirty = useMemo(() => {
    const saved = lastSavedRef.current;
    if (!saved) return false;
    const currentKeys = [...exceptionKeys].sort().join(" ");
    const savedKeys = [...saved.exceptionKeys].sort().join(" ");
    return mode !== saved.mode || currentKeys !== savedKeys;
  }, [mode, exceptionKeys]);

  const handleSave = useCallback(async () => {
    if (!config) return;
    if (!availableAclPolicy) {
      setStatus({
        message:
          "Access control policy is not available on the active gateway.",
        severity: "error",
      });
      return;
    }
    if (!hasAnyCapability) {
      setStatus({
        message:
          "This proxy has no tools, resources, or prompts to apply access control to.",
        severity: "error",
      });
      return;
    }
    const params = buildAclPolicyParams(
      mode,
      exceptionKeys,
      capabilitiesPresent,
    );
    const newPolicy: MCPProxyPolicy = {
      name: ACL_POLICY_NAME,
      version: availableAclPolicy.version,
      displayName: availableAclPolicy.displayName,
      params,
    };
    const existingPolicies = config.policies ?? [];
    const existingIndex = existingPolicies.findIndex(
      (p) => p.name === ACL_POLICY_NAME,
    );
    const nextPolicies =
      existingIndex >= 0
        ? existingPolicies.map((p, i) => (i === existingIndex ? newPolicy : p))
        : [...existingPolicies, newPolicy];

    try {
      await onUpdate({ policies: nextPolicies });
      lastSavedRef.current = { mode, exceptionKeys: [...exceptionKeys] };
      setStatus({
        message: "Access control updated successfully.",
        severity: "success",
      });
    } catch {
      setStatus({
        message: "Failed to update access control.",
        severity: "error",
      });
    }
  }, [
    config,
    availableAclPolicy,
    hasAnyCapability,
    mode,
    exceptionKeys,
    capabilitiesPresent,
    onUpdate,
  ]);

  const handleDiscard = useCallback(() => {
    const saved = lastSavedRef.current;
    if (saved) {
      setMode(saved.mode);
      setExceptionKeys([...saved.exceptionKeys]);
    }
    setStatus(null);
  }, []);

  if (isIdentitySecurity) {
    const isRbacDisabled = isLoading || !config;
    const noToolsForRbac =
      identityMode === "rbac" && !isLoading && config && toolEntries.length === 0;

    return (
      <Stack spacing={2}>
        <Stack direction="row" alignItems="center" spacing={1.5}>
          <Typography variant="body1">Mode</Typography>
          <ToggleButtonGroup
            size="small"
            value={identityMode}
            exclusive
            disabled={isRbacDisabled}
            onChange={(_e, value: IdentityAccessMode | null) =>
              value && setIdentityMode(value)
            }
          >
            <ToggleButton color="primary" value="allowAll" sx={{ textTransform: "none" }}>
              Allow All
              <Tooltip arrow title={ALLOW_ALL_TOOLTIP}>
                <IconButton size="small">
                  <HelpCircle size={16} />
                </IconButton>
              </Tooltip>
            </ToggleButton>
            <ToggleButton color="primary" value="rbac" sx={{ textTransform: "none" }}>
              RBAC
              <Tooltip arrow title={RBAC_TOOLTIP}>
                <IconButton size="small">
                  <HelpCircle size={16} />
                </IconButton>
              </Tooltip>
            </ToggleButton>
          </ToggleButtonGroup>
        </Stack>

        {identityMode === "allowAll" && (
          <Alert severity="warning">
            This MCP proxy is not secured. Any caller with a valid Agent
            Identity token can invoke every tool.
          </Alert>
        )}

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
          identityMode === "rbac" && (
        <ListingTable.Container>
          <ListingTable.Toolbar
            actions={
              <Button
                variant="contained"
                startIcon={<Plus size={16} />}
                onClick={handleAddToolScopeRow}
              >
                Add Tool
              </Button>
            }
          />
          {toolScopeRows.length === 0 ? (
            <ListingTable.EmptyState
              illustration={<Wrench size={64} />}
              title="No tool scope bindings yet"
              description='Click "Add Tool" to gate a tool with catalog scopes.'
            />
          ) : (
            <ListingTable>
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell width="30%">Tool</ListingTable.Cell>
                  <ListingTable.Cell>Scopes</ListingTable.Cell>
                  <ListingTable.Cell align="center" width="60px" />
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {toolScopeRows.map((row) => (
                  <ListingTable.Row key={row.id}>
                    <ListingTable.Cell>
                      <Select
                        size="small"
                        displayEmpty
                        fullWidth
                        value={row.tool}
                        onChange={(e) =>
                          handleToolScopeRowToolChange(
                            row.id,
                            e.target.value as string,
                          )
                        }
                        renderValue={(value) => (value as string) || "Select a tool"}
                        sx={{ minWidth: 200 }}
                      >
                        {toolEntries.map((entry) => (
                          <MenuItem key={entry.identifier} value={entry.identifier}>
                            {entry.identifier}
                          </MenuItem>
                        ))}
                      </Select>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Autocomplete
                        multiple
                        size="small"
                        disableCloseOnSelect
                        options={catalogScopes}
                        value={row.scopes}
                        loading={creatingScopeRowId === row.id}
                        disabled={creatingScopeRowId === row.id}
                        onChange={(_e, value) =>
                          handleToolScopeRowScopesChange(
                            row.id,
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
                            (option) => option.name === inputValue,
                          );
                          if (inputValue.length > 0 && !exists) {
                            filtered.push({
                              id: NEW_SCOPE_OPTION_ID,
                              name: inputValue,
                              isNew: true,
                            });
                          }
                          return filtered;
                        }}
                        getOptionLabel={(option) => (option as ScopeOption).name}
                        isOptionEqualToValue={(option, value) =>
                          (option as ScopeOption).name ===
                          (value as ScopeOption).name
                        }
                        renderOption={(props, option) => {
                          const scopeOption = option as ScopeOption;
                          return (
                            <li {...props} key={scopeOption.id}>
                              {scopeOption.isNew
                                ? `+ Add Scope "${scopeOption.name}"`
                                : scopeOption.name}
                            </li>
                          );
                        }}
                        renderTags={(value, getTagProps) =>
                          value.map((option, index) => (
                            <Chip
                              {...getTagProps({ index })}
                              key={option.name}
                              label={option.name}
                              size="small"
                            />
                          ))
                        }
                        renderInput={(params) => (
                          <TextField
                            {...params}
                            placeholder="Add scopes..."
                            slotProps={{
                              input: {
                                ...params.InputProps,
                                endAdornment: (
                                  <>
                                    {creatingScopeRowId === row.id ? (
                                      <CircularProgress
                                        color="inherit"
                                        size={16}
                                      />
                                    ) : null}
                                    {params.InputProps.endAdornment}
                                  </>
                                ),
                              },
                            }}
                          />
                        )}
                        noOptionsText="No scopes in the catalog"
                        sx={{ minWidth: 280 }}
                      />
                    </ListingTable.Cell>
                    <ListingTable.Cell align="center">
                      <Tooltip title="Remove binding">
                        <IconButton
                          size="small"
                          onClick={() => handleRemoveToolScopeRow(row.id)}
                        >
                          <Trash size={16} />
                        </IconButton>
                      </Tooltip>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                ))}
              </ListingTable.Body>
            </ListingTable>
          )}
        </ListingTable.Container>
          )
        )}
        <Stack spacing={1.5} width="100%">
          <Collapse in={!!toolScopesError} timeout={300}>
            {toolScopesError && (
              <Alert
                severity="error"
                onClose={() => setToolScopesError(undefined)}
                sx={{ width: "100%", maxWidth: 480 }}
              >
                {toolScopesError}
              </Alert>
            )}
          </Collapse>
          <Stack direction="row" spacing={1.5} justifyContent="flex-end">
            <Button
              variant="outlined"
              onClick={handleDiscardToolScopes}
              disabled={!toolScopesDirty || isUpdating}
            >
              Discard
            </Button>
            <Button
              variant="contained"
              onClick={() => void handleSaveToolScopes()}
              disabled={!toolScopesDirty || isUpdating}
            >
              {isUpdating ? "Saving..." : "Save"}
            </Button>
          </Stack>
        </Stack>
      </Stack>
    );
  }

  if (!isLoading && config && !hasAnyCapability) {
    return (
      <Stack
        alignItems="center"
        justifyContent="center"
        spacing={1}
        sx={{ minHeight: 200, textAlign: "center" }}
      >
        <Typography variant="subtitle1" fontWeight={600}>
          No Capabilities Available
        </Typography>
        <Typography variant="body2" color="text.secondary">
          This MCP proxy has no tools, resources, or prompts. Access control
          rules require at least one capability.
        </Typography>
      </Stack>
    );
  }

  return (
    <Stack spacing={2}>
      {!isCatalogLoading && !availableAclPolicy && (
        <Alert severity="warning">
          The access control policy ({ACL_POLICY_NAME}) is not reported as
          available by the active gateway. Saving is disabled.
        </Alert>
      )}
      <AccessControlPanel
        items={items}
        mode={mode}
        onModeChange={setMode}
        exceptionKeys={exceptionKeys}
        onExceptionKeysChange={setExceptionKeys}
        isLoading={isLoading || isCatalogLoading}
        isSaving={isUpdating}
        isDirty={isDirty && Boolean(availableAclPolicy)}
        onSave={() => void handleSave()}
        onDiscard={handleDiscard}
        status={status}
        onClearStatus={() => setStatus(null)}
        availableEmptyTitle="No available capabilities"
        availableEmptyDescription="Tools, resources, and prompts will appear here once the proxy reports them."
      />
    </Stack>
  );
}
