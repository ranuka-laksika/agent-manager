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
import type { MCPProxy } from "@agent-management-platform/types";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Button,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  IconButton,
  InputAdornment,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChevronDown, Eye, EyeOff, HelpCircle } from "@wso2/oxygen-ui-icons-react";
import { validateEndpointUrl } from "@agent-management-platform/shared-component";

const MASKED_CREDENTIAL_VALUE = "••••••••••••";

export type MCPProxyConnectionTabProps = {
  proxy: MCPProxy | null | undefined;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPProxy>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxyConnectionTab({
  proxy,
  isLoading = false,
  onUpdate,
  isUpdating,
}: MCPProxyConnectionTabProps) {
  const theme = useTheme();
  const initializedProxyIdRef = useRef<string | null>(null);

  const [endpoint, setEndpoint] = useState("");
  const [authHeader, setAuthHeader] = useState("");
  const [credentialValue, setCredentialValue] = useState("");
  const [isCredentialMasked, setIsCredentialMasked] = useState(false);
  const [showCredential, setShowCredential] = useState(false);
  const [endpointError, setEndpointError] = useState<string | null>(null);
  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);

  // Header and credential are stored together, so an existing header implies a
  // stored credential (the value itself is never returned by the backend).
  const hasStoredCredential = useMemo(
    () => Boolean(proxy?.upstream?.main?.auth?.header),
    [proxy?.upstream?.main?.auth?.header],
  );

  const resetFromProxy = useCallback(() => {
    if (!proxy) return;
    setEndpoint(proxy.upstream?.main?.url ?? "");
    setAuthHeader(proxy.upstream?.main?.auth?.header ?? "");
    const hasCredential = Boolean(proxy.upstream?.main?.auth?.header);
    setCredentialValue(hasCredential ? MASKED_CREDENTIAL_VALUE : "");
    setIsCredentialMasked(hasCredential);
    setShowCredential(false);
    setEndpointError(null);
  }, [proxy]);

  useEffect(() => {
    if (!proxy) return;
    if (initializedProxyIdRef.current === proxy.id) return;
    initializedProxyIdRef.current = proxy.id;
    resetFromProxy();
  }, [proxy, resetFromProxy]);

  const validateEndpoint = useCallback((value: string): string | null => {
    const err = validateEndpointUrl(value, {
      requiredMessage: "MCP Proxy Endpoint URL is required",
    });
    setEndpointError(err);
    return err;
  }, []);

  const credentialChanged =
    !isCredentialMasked && credentialValue.trim() !== MASKED_CREDENTIAL_VALUE;

  const isDirty = useMemo(() => {
    if (!proxy) return false;
    const savedUrl = (proxy.upstream?.main?.url ?? "").trim();
    const savedHeader = (proxy.upstream?.main?.auth?.header ?? "").trim();
    if (endpoint.trim() !== savedUrl) return true;
    if (authHeader.trim() !== savedHeader) return true;
    if (credentialChanged) return true;
    return false;
  }, [proxy, endpoint, authHeader, credentialChanged]);

  const handleDiscard = useCallback(() => {
    resetFromProxy();
    setStatus(null);
  }, [resetFromProxy]);

  const handleSave = useCallback(async () => {
    if (!proxy) return;

    const endpointValidationError = validateEndpoint(endpoint);
    if (endpointValidationError) {
      setStatus({ message: endpointValidationError, severity: "error" });
      return;
    }

    const trimmedHeader = authHeader.trim();
    const existingAuth = proxy.upstream?.main?.auth;
    // Preserve any existing auth (including its type); only override header,
    // and value when the user typed a new one — otherwise the backend keeps
    // the stored credential.
    const auth = trimmedHeader
      ? {
          type: "api-key" as const,
          ...existingAuth,
          header: trimmedHeader,
          ...(credentialChanged ? { value: credentialValue.trim() } : {}),
        }
      : undefined;

    try {
      await onUpdate({
        upstream: {
          ...proxy.upstream,
          main: {
            ...proxy.upstream?.main,
            url: endpoint.trim(),
            auth,
          },
        },
      });
      setStatus({
        message: "Connection updated successfully.",
        severity: "success",
      });
      if (credentialChanged) {
        setCredentialValue(MASKED_CREDENTIAL_VALUE);
        setIsCredentialMasked(true);
      }
    } catch {
      setStatus({ message: "Failed to update connection.", severity: "error" });
    }
  }, [
    proxy,
    endpoint,
    authHeader,
    credentialChanged,
    credentialValue,
    onUpdate,
    validateEndpoint,
  ]);

  if (isLoading) {
    return (
      <Stack spacing={2}>
        {[1, 2, 3].map((i) => (
          <Stack key={i} spacing={0.5}>
            <Skeleton variant="text" width={160} height={16} />
            <Skeleton variant="rounded" height={40} />
          </Stack>
        ))}
      </Stack>
    );
  }

  if (!proxy) {
    return null;
  }

  return (
    <Stack spacing={2}>
      <Grid container spacing={3}>
        <Grid size={{ xs: 12 }}>
          <FormControl fullWidth>
            <FormLabel required>MCP Proxy Endpoint URL</FormLabel>
            <TextField
              size="small"
              value={endpoint}
              onChange={(e) => {
                setEndpoint(e.target.value);
                if (endpointError) validateEndpoint(e.target.value);
              }}
              onBlur={() => validateEndpoint(endpoint)}
              error={!!endpointError}
              helperText={endpointError}
              placeholder="Enter URL of your MCP Proxy"
              sx={{
                "& .MuiInputBase-input": {
                  fontFamily: "monospace",
                  fontSize: theme.typography.body2?.fontSize,
                },
              }}
            />
          </FormControl>
        </Grid>

        <Grid size={{ xs: 12 }}>
          <Accordion defaultExpanded disableGutters variant="outlined">
            <AccordionSummary expandIcon={<ChevronDown size={18} />}>
              <Stack direction="row" alignItems="center" spacing={1}>
                <Typography variant="subtitle1" fontWeight={600}>
                  Advanced Configurations
                </Typography>
                <Tooltip title="Configure an optional authentication header sent to the MCP proxy endpoint.">
                  <HelpCircle size={16} />
                </Tooltip>
              </Stack>
            </AccordionSummary>
            <AccordionDetails>
              <Stack spacing={2}>
                <Typography variant="subtitle2" fontWeight={600}>
                  Configure Authentication Header
                </Typography>
                <Grid container spacing={2}>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <FormControl fullWidth>
                      <FormLabel>Header</FormLabel>
                      <TextField
                        size="small"
                        value={authHeader}
                        onChange={(e) => setAuthHeader(e.target.value)}
                        placeholder="Header"
                      />
                    </FormControl>
                  </Grid>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <FormControl fullWidth>
                      <FormLabel>Value</FormLabel>
                      <TextField
                        size="small"
                        type={showCredential ? "text" : "password"}
                        value={credentialValue}
                        placeholder="Value"
                        onFocus={() => {
                          if (isCredentialMasked) {
                            setCredentialValue("");
                            setIsCredentialMasked(false);
                          }
                        }}
                        onChange={(e) => setCredentialValue(e.target.value)}
                        slotProps={{
                          input: {
                            endAdornment: (
                              <InputAdornment position="end">
                                <IconButton
                                  size="small"
                                  onClick={() => setShowCredential((p) => !p)}
                                  aria-label={
                                    showCredential
                                      ? "Hide header value"
                                      : "Show header value"
                                  }
                                  edge="end"
                                >
                                  {showCredential ? (
                                    <EyeOff size={18} />
                                  ) : (
                                    <Eye size={18} />
                                  )}
                                </IconButton>
                              </InputAdornment>
                            ),
                          },
                        }}
                        sx={{ "& .MuiInputBase-input": { fontFamily: "monospace" } }}
                      />
                    </FormControl>
                  </Grid>
                </Grid>
                {hasStoredCredential && (
                  <Typography variant="caption" color="text.secondary">
                    The stored value is hidden. Leave it unchanged to keep the
                    current credential, or enter a new value to replace it.
                  </Typography>
                )}
              </Stack>
            </AccordionDetails>
          </Accordion>
        </Grid>

        <Grid size={{ xs: 12 }}>
          <Stack spacing={1.5} width="100%">
            <Collapse in={!!status} timeout={300}>
              {status && (
                <Alert
                  severity={status.severity}
                  onClose={() => setStatus(null)}
                  sx={{ width: "100%" }}
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
                disabled={isUpdating || !isDirty || !!endpointError}
              >
                {isUpdating ? "Saving..." : "Save"}
              </Button>
            </Stack>
          </Stack>
        </Grid>
      </Grid>
    </Stack>
  );
}

export default MCPProxyConnectionTab;
