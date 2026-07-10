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

import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  APIKeyLocation,
  MCPEndpointConfig,
  MCPProxy,
} from "@agent-management-platform/types";
import {
  Alert,
  Button,
  Collapse,
  FormControl,
  FormLabel,
  Grid,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  type AuthenticationType,
  getAuthenticationTypeLabel,
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

export type MCPProxySecurityTabProps = {
  config: MCPEndpointConfig | undefined;
  selectedEndpointId: string;
  isLoading?: boolean;
  onUpdate: (fields: Partial<MCPEndpointConfig>) => Promise<MCPProxy>;
  isUpdating: boolean;
};

export function MCPProxySecurityTab({
  config,
  selectedEndpointId,
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

  const isDirty = useMemo(() => {
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
      config.security?.apiKey?.key ?? (nextType === "apiKey" ? "X-API-Key" : ""),
    );
    setKeyIn((config.security?.apiKey?.in as APIKeyLocation) ?? "header");
    setFieldErrors({});
  }, [config, selectedEndpointId]);

  const handleDiscard = useCallback(() => {
    if (!config) return;
    const nextType = resolveAuthenticationType(config);
    setAuthenticationType(nextType);
    setKeyValue(
      config.security?.apiKey?.key ?? (nextType === "apiKey" ? "X-API-Key" : ""),
    );
    setKeyIn((config.security?.apiKey?.in as APIKeyLocation) ?? "header");
    setFieldErrors({});
    setStatus(null);
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

    const nextKey = keyValue.trim();
    const nextIn = keyIn;

    try {
      await onUpdate({
        security: {
          enabled: config.security?.enabled ?? true,
          apiKey: {
            enabled: authenticationType === "apiKey",
            key: authenticationType === "apiKey" ? nextKey : "",
            in: nextIn,
          },
          identity: {
            enabled: authenticationType === "identity",
          },
        },
      });
      setFieldErrors({});
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
  }, [config, authenticationType, keyValue, keyIn, onUpdate]);

  const isDisabled = isLoading || !config;

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
            <FormLabel>Authentication Method</FormLabel>
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
        <Alert severity="info">
          Scopes for individual tools can be configured in the Access Control
          tab.
        </Alert>
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

export default MCPProxySecurityTab;
