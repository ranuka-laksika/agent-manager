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
import { debounce } from "lodash";
import { useFetchMCPProxyServerInfo } from "@agent-management-platform/api-client";
import {
  type Environment,
  type MCPServerInfoFetchResponse,
} from "@agent-management-platform/types";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Autocomplete,
  Box,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  Divider,
  Form,
  FormControl,
  FormLabel,
  IconButton,
  InputAdornment,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  Eye,
  EyeOff,
  HelpCircle,
} from "@wso2/oxygen-ui-icons-react";
import { useSnackBar } from "@agent-management-platform/views";
import { validateEndpointUrl } from "@agent-management-platform/shared-component";
import { MCPCapabilitiesView } from "../components/MCPCapabilitiesView";

// EndpointDraft is a single endpoint captured in the form. Its `id` maps to the backend
// endpoint handle (unique within the parent proxy); a fresh draft carries a temporary
// client id that the save path replaces with a handle derived from `name`/URL.
export interface EndpointDraft {
  id: string;
  // Human-readable endpoint name; the backend handle is derived from it when empty.
  name: string;
  url: string;
  authHeader: string;
  authValue: string;
  // Environment UUIDs (not names) this endpoint serves — stable across renames.
  environments: string[];
  fetchedInfo: MCPServerInfoFetchResponse;
  serverName?: string;
  serverVersion?: string;
}

// Sentinel shown in place of a stored auth value (never returned by the API). While
// this is present untouched, the endpoint keeps its existing credential.
const MASKED_CREDENTIAL_VALUE = "••••••••••••";

// How long to wait after the last URL/auth edit before auto-fetching server info,
// so a fetch isn't fired on every keystroke.
const FETCH_DEBOUNCE_MS = 600;

export interface EndpointFormFieldsProps {
  orgId: string;
  // Environments not yet claimed by another endpoint. One environment can be used once.
  // In edit mode this must also include the edited endpoint's own environments.
  availableEnvironments: Environment[];
  onAdd: (endpoint: Omit<EndpointDraft, "id">) => void;
  onCancel: () => void;
  // When provided, the form edits an existing endpoint: fields are pre-filled and
  // its stored (unreadable) credential is masked until the user replaces it.
  initialDraft?: EndpointDraft;
}

export function EndpointFormFields({
  orgId,
  availableEnvironments,
  onAdd,
  onCancel,
  initialDraft,
}: EndpointFormFieldsProps) {
  const fetchServerInfo = useFetchMCPProxyServerInfo();
  const { pushSnackBar } = useSnackBar();

  const isEditing = Boolean(initialDraft);
  const hasStoredCredential = Boolean(initialDraft?.authHeader);

  const [name, setName] = useState(initialDraft?.name ?? "");
  const [url, setUrl] = useState(initialDraft?.url ?? "");
  const [authHeader, setAuthHeader] = useState(initialDraft?.authHeader ?? "");
  const [authValue, setAuthValue] = useState(
    hasStoredCredential ? MASKED_CREDENTIAL_VALUE : "",
  );
  const [isCredentialMasked, setIsCredentialMasked] =
    useState(hasStoredCredential);
  const [showAuthValue, setShowAuthValue] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(hasStoredCredential);
  const [selectedEnvIds, setSelectedEnvIds] = useState<string[]>(
    initialDraft?.environments ?? [],
  );
  const [fetchedInfo, setFetchedInfo] =
    useState<MCPServerInfoFetchResponse | null>(
      initialDraft?.fetchedInfo ?? null,
    );
  const [urlError, setUrlError] = useState<string | null>(null);
  const [authError, setAuthError] = useState<string | null>(null);

  const trimmedUrl = url.trim();
  const isFetched = Boolean(fetchedInfo);
  const isFetching = fetchServerInfo.isPending;
  const canAdd = isFetched && selectedEnvIds.length > 0;
  // In edit mode, Save stays disabled until the user actually changes something; a
  // brand-new endpoint (no initialDraft) is always a change. The credential counts as
  // changed only once the masked stored value has been replaced, and a re-fetch counts
  // when it returns different capabilities (e.g. newly added tools).
  const hasChanges =
    !initialDraft ||
    name.trim() !== initialDraft.name ||
    trimmedUrl !== initialDraft.url ||
    authHeader.trim() !== initialDraft.authHeader ||
    (!isCredentialMasked && authValue.trim().length > 0) ||
    !sameIdSet(selectedEnvIds, initialDraft.environments) ||
    capabilitiesChanged(fetchedInfo, initialDraft.fetchedInfo);
  const canSave = canAdd && hasChanges;

  const clearFetched = useCallback(() => {
    setFetchedInfo(null);
  }, []);

  const performFetch = useCallback(async () => {
    // Guards against a debounced call landing while a previous fetch for this form
    // is still in flight (e.g. a slow request outlasting the debounce window).
    if (fetchServerInfo.isPending) return;

    const urlValidationError = validateEndpointUrl(trimmedUrl, {
      requiredMessage: "Enter a valid MCP Proxy endpoint URL.",
      invalidMessage: "Enter a valid MCP Proxy endpoint URL.",
      protocolMessage: "Enter a valid MCP Proxy endpoint URL.",
    });
    if (urlValidationError) {
      setUrlError(urlValidationError);
      return;
    }
    setUrlError(null);

    const header = authHeader.trim();
    // The stored credential is never returned, so it can't be replayed to the
    // live fetch — ask the user to re-enter it before re-fetching.
    if (header && isCredentialMasked) {
      setIsCredentialMasked(false);
      setAuthValue("");
      setAdvancedOpen(true);
      setAuthError(
        "Re-enter the authentication value to re-fetch server info.",
      );
      return;
    }
    const value = authValue.trim();
    if (Boolean(header) !== Boolean(value)) {
      setAdvancedOpen(true);
      setAuthError("Enter both an authentication header and value.");
      return;
    }
    setAuthError(null);

    const body =
      header && value
        ? { url: trimmedUrl, auth: { type: "api-key" as const, header, value } }
        : { url: trimmedUrl };

    try {
      const result = await fetchServerInfo.mutateAsync({
        params: { orgName: orgId },
        body,
      });
      setFetchedInfo(result);
    } catch (err: unknown) {
      setFetchedInfo(null);
      if (
        typeof err === "object" &&
        err !== null &&
        (err as { code?: string }).code === "UNAUTHORIZED"
      ) {
        setAdvancedOpen(true);
        setAuthError(
          "This server requires authentication. Enter the credentials above.",
        );
      } else {
        const message =
          err instanceof Error
            ? err.message
            : "Failed to fetch MCP server info. Please check the URL and try again.";
        pushSnackBar({ message, type: "error" });
      }
    }
  }, [authHeader, authValue, fetchServerInfo, orgId, pushSnackBar, trimmedUrl]);

  // Auto-fetch server info shortly after the URL or auth fields settle, instead of
  // requiring an explicit "Fetch" click. `performFetch` is re-created on every render
  // in which `fetchServerInfo` (the mutation object) changes identity — which react-query
  // does on its own pending/success/error transitions, not just when the form's inputs
  // change. Triggering off `performFetch` directly would re-schedule the debounce after
  // every fetch completes, causing a fetch loop. A ref sidesteps that: the effect that
  // schedules the debounce only depends on the actual field values, while the debounced
  // call always reads the latest `performFetch` out of the ref.
  const performFetchRef = useRef(performFetch);
  useEffect(() => {
    performFetchRef.current = performFetch;
  }, [performFetch]);

  const debouncedFetch = useMemo(
    () => debounce(() => void performFetchRef.current(), FETCH_DEBOUNCE_MS),
    [],
  );
  useEffect(() => () => debouncedFetch.cancel(), [debouncedFetch]);

  // Skip the very first run: on mount, an edited endpoint's fields are seeded from
  // `initialDraft` along with its already-fetched `fetchedInfo`, so re-fetching would
  // just repeat a request that already has a correct answer. Only field edits made
  // after mount should schedule a re-fetch.
  const skippedInitialFetchTrigger = useRef(false);
  useEffect(() => {
    if (!skippedInitialFetchTrigger.current) {
      skippedInitialFetchTrigger.current = true;
      return;
    }
    if (!trimmedUrl) return;
    debouncedFetch();
  }, [trimmedUrl, authHeader, authValue, debouncedFetch]);

  const handleAdd = useCallback(() => {
    if (!fetchedInfo || selectedEnvIds.length === 0) return;
    // An untouched masked credential means "keep the stored value" — emit an empty
    // authValue so the save path omits it and the backend preserves the secret.
    const resolvedAuthValue = isCredentialMasked ? "" : authValue.trim();
    onAdd({
      name: name.trim(),
      url: trimmedUrl,
      authHeader: authHeader.trim(),
      authValue: resolvedAuthValue,
      environments: selectedEnvIds,
      fetchedInfo,
      serverName:
        getServerInfoValue(fetchedInfo.serverInfo, "name") ??
        initialDraft?.serverName,
      serverVersion:
        getServerInfoValue(fetchedInfo.serverInfo, "version") ??
        initialDraft?.serverVersion,
    });
  }, [
    authHeader,
    authValue,
    fetchedInfo,
    initialDraft,
    isCredentialMasked,
    name,
    onAdd,
    selectedEnvIds,
    trimmedUrl,
  ]);

  const serverName = fetchedInfo
    ? getServerInfoValue(fetchedInfo.serverInfo, "name")
    : undefined;
  const serverVersion = fetchedInfo
    ? getServerInfoValue(fetchedInfo.serverInfo, "version")
    : undefined;

  return (
    <Form.Stack spacing={2.5}>
      <Typography variant="body2" color="text.secondary">
        Point to your MCP server and choose the environments this endpoint
        serves. Capabilities are fetched automatically once the URL is valid.
      </Typography>

      <FormControl fullWidth>
        <FormLabel>Endpoint Name</FormLabel>
        <TextField
          fullWidth
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="Primary"
          helperText="Optional. A handle is derived from the name (or URL) when left blank."
        />
      </FormControl>

      <FormControl fullWidth error={Boolean(urlError)}>
        <FormLabel required>MCP Proxy Endpoint URL</FormLabel>
        <TextField
          fullWidth
          value={url}
          onChange={(event) => {
            setUrl(event.target.value);
            clearFetched();
            setUrlError(null);
          }}
          placeholder="Enter URL of your MCP Proxy"
          error={Boolean(urlError)}
          helperText={urlError}
        />
      </FormControl>

      <Accordion
        expanded={advancedOpen}
        onChange={(_, expanded) => setAdvancedOpen(expanded)}
        disableGutters
        variant="outlined"
      >
        <AccordionSummary expandIcon={<ChevronDown size={18} />}>
          <Stack direction="row" alignItems="center" spacing={1}>
            <Typography variant="subtitle2" fontWeight={600}>
              Advanced Configurations
            </Typography>
            <Tooltip title="Configure an optional authentication header sent to the MCP server.">
              <HelpCircle size={16} />
            </Tooltip>
          </Stack>
        </AccordionSummary>
        <AccordionDetails>
          <Form.Stack spacing={2}>
            <Typography variant="subtitle2" fontWeight={600}>
              Configure Authentication Header
            </Typography>
            <Form.Stack
              direction={{ xs: "column", md: "row" }}
              spacing={2}
              useFlexGap
            >
              <FormControl sx={{ flex: 1 }} error={Boolean(authError)}>
                <FormLabel>Header</FormLabel>
                <TextField
                  fullWidth
                  value={authHeader}
                  onChange={(event) => {
                    setAuthHeader(event.target.value);
                    clearFetched();
                    setAuthError(null);
                  }}
                  placeholder="Header"
                  error={Boolean(authError)}
                />
              </FormControl>
              <FormControl sx={{ flex: 1 }} error={Boolean(authError)}>
                <FormLabel>Value</FormLabel>
                <TextField
                  fullWidth
                  value={authValue}
                  onFocus={() => {
                    // Reveal a blank field so the user replaces the hidden
                    // stored credential rather than editing the mask.
                    if (isCredentialMasked) {
                      setAuthValue("");
                      setIsCredentialMasked(false);
                      clearFetched();
                    }
                  }}
                  onChange={(event) => {
                    setAuthValue(event.target.value);
                    setIsCredentialMasked(false);
                    clearFetched();
                    setAuthError(null);
                  }}
                  placeholder="Value"
                  error={Boolean(authError)}
                  helperText={
                    authError ??
                    (isCredentialMasked
                      ? "Leave unchanged to keep the stored value."
                      : undefined)
                  }
                  type={showAuthValue ? "text" : "password"}
                  slotProps={{
                    input: {
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            aria-label={
                              showAuthValue
                                ? "Hide header value"
                                : "Show header value"
                            }
                            onClick={() => setShowAuthValue((prev) => !prev)}
                            edge="end"
                          >
                            {showAuthValue ? (
                              <EyeOff size={18} />
                            ) : (
                              <Eye size={18} />
                            )}
                          </IconButton>
                        </InputAdornment>
                      ),
                    },
                  }}
                />
              </FormControl>
            </Form.Stack>
          </Form.Stack>
        </AccordionDetails>
      </Accordion>

      <FormControl fullWidth>
        <FormLabel required>Environments</FormLabel>
        <Autocomplete
          multiple
          options={availableEnvironments}
          size="small"
          value={availableEnvironments.filter(
            (env) => env.id != null && selectedEnvIds.includes(env.id),
          )}
          onChange={(_, selected) =>
            setSelectedEnvIds(
              selected
                .map((env) => env.id)
                .filter((id): id is string => Boolean(id)),
            )
          }
          getOptionLabel={(option) => option.displayName || option.name}
          isOptionEqualToValue={(option, value) => option.id === value.id}
          renderInput={(params) => (
            <TextField
              {...params}
              placeholder={
                availableEnvironments.length === 0
                  ? "All environments are already assigned"
                  : "Select environment(s)"
              }
            />
          )}
          disabled={availableEnvironments.length === 0}
          sx={{ mt: 0.5 }}
        />
        <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
          An environment can be assigned to only one endpoint.
        </Typography>
      </FormControl>

      <Collapse in={isFetching || isFetched} timeout="auto" unmountOnExit>
        <Stack spacing={2}>
          <Divider />
          {isFetching ? (
            <Stack direction="row" spacing={1.5} alignItems="center">
              <CircularProgress size={18} />
              <Typography variant="body2" color="text.secondary">
                Fetching server info...
              </Typography>
            </Stack>
          ) : fetchedInfo ? (
            <Stack spacing={2}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="h6" fontWeight={600}>
                  {serverName || "MCP Server"}
                </Typography>
                {serverVersion ? (
                  <Chip
                    label={
                      serverVersion.startsWith("v")
                        ? serverVersion
                        : `v${serverVersion}`
                    }
                    size="small"
                    variant="outlined"
                  />
                ) : null}
              </Stack>
              <MCPCapabilitiesView
                tools={fetchedInfo.tools}
                resources={fetchedInfo.resources}
                prompts={fetchedInfo.prompts}
              />
            </Stack>
          ) : null}
        </Stack>
      </Collapse>

      <Box display="flex" justifyContent="flex-end" gap={1}>
        <Button variant="outlined" onClick={onCancel} disabled={isFetching}>
          Cancel
        </Button>
        {/* Add/Update requires a completed fetch (canAdd depends on isFetched), so
            any field change that clears the fetch disables it until the debounced
            re-fetch completes. */}
        <Button
          variant="contained"
          onClick={handleAdd}
          disabled={!canSave || isFetching}
        >
          {isEditing ? "Update Endpoint" : "Add Endpoint"}
        </Button>
      </Box>
    </Form.Stack>
  );
}

function getServerInfoValue(
  serverInfo: Record<string, unknown> | undefined,
  key: string,
): string | undefined {
  const value = serverInfo?.[key];
  return typeof value === "string" ? value : undefined;
}

// Order-insensitive equality for the selected environment IDs.
function sameIdSet(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false;
  const setB = new Set(b);
  return a.every((id) => setB.has(id));
}

// Whether a re-fetch produced capabilities that differ from the stored ones. Only the
// tool/resource/prompt lists are compared, since those are what gets persisted.
function capabilitiesChanged(
  current: MCPServerInfoFetchResponse | null,
  original: MCPServerInfoFetchResponse,
): boolean {
  if (!current) return false;
  return (
    JSON.stringify(current.tools ?? []) !==
      JSON.stringify(original.tools ?? []) ||
    JSON.stringify(current.resources ?? []) !==
      JSON.stringify(original.resources ?? []) ||
    JSON.stringify(current.prompts ?? []) !==
      JSON.stringify(original.prompts ?? [])
  );
}
