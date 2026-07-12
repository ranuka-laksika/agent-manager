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
  useCreateScope,
  useListScopes,
} from "@agent-management-platform/api-client";
import type {
  APIKeyLocation,
  MCPEndpointConfig,
  MCPProxy,
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
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  ListingTable,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Info, Plus, Trash, Wrench } from "@wso2/oxygen-ui-icons-react";
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
// entry stands in for "create this scope and add it" — it never reaches
// row.scopes, so `id` only needs to be present to satisfy ScopeResponse.
type ScopeOption = ScopeResponse & { isNew?: boolean };

const NEW_SCOPE_OPTION_ID = "__new_scope__";
const filterScopeOptions = createFilterOptions<ScopeOption>();

// A local editable row for the identity-security tool-scope-binding table.
// Keyed by a local id (not the tool name) since the same tool can appear in
// more than one binding.
type ToolScopeRow = {
  id: number;
  tool: string;
  scopes: ScopeResponse[];
};

export type MCPProxySecurityTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  orgName: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxySecurityTab({
  config,
  selectedEndpointId,
  orgName,
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

  const authIsDirty = useMemo(() => {
    if (!config) return false;
    const savedType = resolveAuthenticationType(config);
    const savedKey = config.security?.apiKey?.key ?? "";
    const savedIn = (config.security?.apiKey?.in as APIKeyLocation) ?? "header";
    if (authenticationType !== savedType) return true;
    if (keyValue.trim() !== savedKey) return true;
    if (keyIn !== savedIn) return true;
    return false;
  }, [config, authenticationType, keyValue, keyIn]);

  useEffect(() => {
    if (!config || !selectedEndpointId) return;
    const nextType = resolveAuthenticationType(config);
    setAuthenticationType(nextType);
    setKeyValue(
      config.security?.apiKey?.key ??
      (nextType === "apiKey" ? "X-API-Key" : ""),
    );
    setKeyIn((config.security?.apiKey?.in as APIKeyLocation) ?? "header");
    setFieldErrors({});
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
    [toolEntries, config],
  );

  const { data: scopesData } = useListScopes(
    { orgName },
    { enabled: authenticationType === "identity" },
  );
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
    setToolScopeRows(rows);
    lastSavedToolScopeRowsRef.current = rows;
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
    return (
      serializeToolScopeRows(toolScopeRows) !==
      serializeToolScopeRows(lastSavedToolScopeRowsRef.current)
    );
  }, [toolScopeRows]);

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
          setCreatingScopeRowId((current) =>
            current === rowId ? null : current,
          );
        });
    },
    [orgName, createScope, setRowScopes],
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
    setToolScopesError(undefined);
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

    if (
      authenticationType === "identity" &&
      toolScopeRows.some((row) => !row.tool || row.scopes.length === 0)
    ) {
      setToolScopesError(
        "Every row needs a tool and at least one scope before saving.",
      );
      return;
    }
    setToolScopesError(undefined);

    // Scope bindings only apply under Agent Identity security — switching to
    // API Key (or no) authentication clears them rather than resending
    // whatever was last mirrored locally in toolScopeRows.
    const savedToolScopeRows =
      authenticationType === "identity" ? toolScopeRows : [];
    const nextBindings: MCPToolScopeBinding[] = savedToolScopeRows.map(
      (row) => ({
        tool: row.tool,
        scopes: row.scopes.map((s) => s.name),
      }),
    );

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
        toolScopeBindings: nextBindings,
      });
      lastSavedToolScopeRowsRef.current = savedToolScopeRows;
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
  }, [config, authenticationType, keyValue, keyIn, toolScopeRows, onUpdate]);

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
                setAuthenticationType(
                  (e.target.value as AuthenticationType) || "",
                )
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
              <ListingTable.Toolbar
                actions={
                  <Button
                    variant="outlined"
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
                  description='Click "Add Tool" to gate a tool with scopes.'
                />
              ) : (
                <ListingTable density="compact">
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
                            renderValue={(value) => {
                              const identifier = value as string;
                              if (!identifier) return "Select a tool";
                              return (
                                <Stack
                                  direction="row"
                                  alignItems="center"
                                  sx={{ width: "100%" }}
                                  spacing={1}
                                >
                                  <span>{identifier}</span>
                                  {blockedToolIds.has(identifier) && (
                                    <Tooltip title="Blocked by Manage Tools">
                                      <Stack color="warning.main" direction="row" alignItems="center">
                                        <Info size={14} />
                                      </Stack>
                                    </Tooltip>
                                  )}
                                </Stack>
                              );
                            }}
                            sx={{ minWidth: 200 }}
                          >
                            {toolEntries.map((identifier) => (
                              <MenuItem key={identifier} value={identifier}>
                                <Stack
                                  direction="row"
                                  alignItems="center"
                                  spacing={1}
                                  sx={{ width: "100%" }}
                                >
                                  <Typography component="span" variant="body2" noWrap>
                                    {identifier}
                                  </Typography>
                                  {blockedToolIds.has(identifier) && (
                                    <Tooltip title="Blocked by Manage Tools">
                                      <Stack color="warning.main" direction="row" alignItems="center">
                                        <Info size={14} />
                                      </Stack>
                                    </Tooltip>
                                  )}

                                </Stack>
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
                            getOptionLabel={(option) =>
                              (option as ScopeOption).name
                            }
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
          )}
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
