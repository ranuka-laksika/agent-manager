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

import { useCallback, useEffect, useState } from "react";
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
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
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

// EndpointDraft is a single per-environment upstream endpoint captured in the form.
export interface EndpointDraft {
  id: string;
  url: string;
  authHeader: string;
  authValue: string;
  // Environment UUIDs (not names) this endpoint serves — stable across renames.
  environments: string[];
  fetchedInfo: MCPServerInfoFetchResponse;
  serverName?: string;
  serverVersion?: string;
}

interface AddEndpointDialogProps {
  open: boolean;
  orgId: string;
  // Environments not yet claimed by another endpoint. One environment can be used once.
  availableEnvironments: Environment[];
  onClose: () => void;
  onAdd: (endpoint: Omit<EndpointDraft, "id">) => void;
}

export function AddEndpointDialog({
  open,
  orgId,
  availableEnvironments,
  onClose,
  onAdd,
}: AddEndpointDialogProps) {
  const fetchServerInfo = useFetchMCPProxyServerInfo();
  const { pushSnackBar } = useSnackBar();

  const [url, setUrl] = useState("");
  const [authHeader, setAuthHeader] = useState("");
  const [authValue, setAuthValue] = useState("");
  const [showAuthValue, setShowAuthValue] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [selectedEnvIds, setSelectedEnvIds] = useState<string[]>([]);
  const [fetchedInfo, setFetchedInfo] =
    useState<MCPServerInfoFetchResponse | null>(null);
  const [urlError, setUrlError] = useState<string | null>(null);
  const [authError, setAuthError] = useState<string | null>(null);

  const resetState = useCallback(() => {
    setUrl("");
    setAuthHeader("");
    setAuthValue("");
    setShowAuthValue(false);
    setAdvancedOpen(false);
    setSelectedEnvIds([]);
    setFetchedInfo(null);
    setUrlError(null);
    setAuthError(null);
  }, []);

  // Start from a clean slate each time the dialog is opened.
  useEffect(() => {
    if (open) resetState();
  }, [open, resetState]);

  const trimmedUrl = url.trim();
  const isFetched = Boolean(fetchedInfo);
  const isFetching = fetchServerInfo.isPending;
  const canAdd = isFetched && selectedEnvIds.length > 0;

  const clearFetched = useCallback(() => {
    setFetchedInfo(null);
  }, []);

  const performFetch = useCallback(async () => {
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

  const handleAdd = useCallback(() => {
    if (!fetchedInfo || selectedEnvIds.length === 0) return;
    onAdd({
      url: trimmedUrl,
      authHeader: authHeader.trim(),
      authValue: authValue.trim(),
      environments: selectedEnvIds,
      fetchedInfo,
      serverName: getServerInfoValue(fetchedInfo.serverInfo, "name"),
      serverVersion: getServerInfoValue(fetchedInfo.serverInfo, "version"),
    });
    resetState();
  }, [
    authHeader,
    authValue,
    fetchedInfo,
    onAdd,
    resetState,
    selectedEnvIds,
    trimmedUrl,
  ]);

  const handleClose = useCallback(() => {
    if (isFetching) return;
    resetState();
    onClose();
  }, [isFetching, onClose, resetState]);

  const serverName = fetchedInfo
    ? getServerInfoValue(fetchedInfo.serverInfo, "name")
    : undefined;
  const serverVersion = fetchedInfo
    ? getServerInfoValue(fetchedInfo.serverInfo, "version")
    : undefined;

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>Add Endpoint</DialogTitle>
      <DialogContent>
        <Form.Stack spacing={2.5} sx={{ mt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Point to your MCP server, fetch its capabilities, and choose the
            environments this endpoint serves.
          </Typography>

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
                      onChange={(event) => {
                        setAuthValue(event.target.value);
                        clearFetched();
                        setAuthError(null);
                      }}
                      placeholder="Value"
                      error={Boolean(authError)}
                      helperText={authError}
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

          <Collapse in={isFetched} timeout="auto" unmountOnExit>
            {fetchedInfo ? (
              <Stack spacing={2}>
                <Divider />
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
          </Collapse>
        </Form.Stack>
      </DialogContent>
      <DialogActions>
        <Button variant="outlined" onClick={handleClose} disabled={isFetching}>
          Cancel
        </Button>
        {isFetched ? (
          <Button variant="contained" onClick={handleAdd} disabled={!canAdd}>
            Add
          </Button>
        ) : (
          <Button
            variant="contained"
            onClick={performFetch}
            disabled={!trimmedUrl || isFetching}
            startIcon={
              isFetching ? (
                <Box component="span" sx={{ display: "inline-flex" }}>
                  <CircularProgress size={16} color="inherit" />
                </Box>
              ) : undefined
            }
          >
            {isFetching ? "Fetching" : "Fetch Server Info"}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
}

function getServerInfoValue(
  serverInfo: Record<string, unknown> | undefined,
  key: string,
): string | undefined {
  const value = serverInfo?.[key];
  return typeof value === "string" ? value : undefined;
}
