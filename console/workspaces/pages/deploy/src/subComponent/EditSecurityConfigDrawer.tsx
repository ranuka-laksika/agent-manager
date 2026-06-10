/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  useGetAgent,
  useGetAgentConfigurations,
  useUpdateAgentDeploySettings,
} from "@agent-management-platform/api-client";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Collapse,
  Form,
  FormControl,
  FormControlLabel,
  FormLabel,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, Shield } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useSnackBar,
} from "@agent-management-platform/views";
import { useCallback, useEffect, useState } from "react";

export interface EditSecurityConfigDrawerProps {
  open: boolean;
  onClose: () => void;
  orgName: string;
  projName: string;
  agentName: string;
  environment: string;
}

export function EditSecurityConfigDrawer({
  open,
  onClose,
  orgName,
  projName,
  agentName,
  environment,
}: EditSecurityConfigDrawerProps) {
  const { pushSnackBar } = useSnackBar();

  const { data: agent } = useGetAgent({ orgName, projName, agentName });
  // Mount the configurations query so its cache invalidates after we save —
  // /deploy-settings doesn't read this, but the page elsewhere does.
  useGetAgentConfigurations(
    { orgName, projName, agentName },
    { environment },
  );

  const isApiAgent = agent?.agentType?.type === "agent-api";

  // ── State ─────────────────────────────────────────────────────────────────
  const [corsEnabled, setCorsEnabled] = useState(false);
  const [corsAllowAll, setCorsAllowAll] = useState(true);
  const [corsOrigins, setCorsOrigins] = useState<string[]>(["*"]);
  const [corsMethods, setCorsMethods] = useState<string[]>(["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"]);
  const [corsHeaders, setCorsHeaders] = useState<string[]>(["authorization", "Content-Type", "Origin", "X-API-Key"]);
  const [corsAllowCredentials, setCorsAllowCredentials] = useState(false);

  // Seed CORS form from agent config when drawer opens. When no persisted
  // config exists, reset to defaults so stale in-memory edits from a previous
  // open don't leak across reopens.
  useEffect(() => {
    if (!open) return;
    const cors = agent?.configurations?.corsConfig;
    if (!cors) {
      setCorsEnabled(false);
      setCorsAllowAll(true);
      setCorsOrigins(["*"]);
      setCorsMethods(["GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"]);
      setCorsHeaders(["authorization", "Content-Type", "Origin", "X-API-Key"]);
      setCorsAllowCredentials(false);
      return;
    }
    if (cors.enabled !== undefined) setCorsEnabled(cors.enabled);
    if (cors.allowOrigin !== undefined) {
      const isWildcard = cors.allowOrigin.length === 1 && cors.allowOrigin[0] === "*";
      setCorsAllowAll(isWildcard);
      setCorsOrigins(cors.allowOrigin);
    }
    if (cors.allowMethods !== undefined) setCorsMethods(cors.allowMethods);
    if (cors.allowHeaders !== undefined) setCorsHeaders(cors.allowHeaders);
    if (cors.allowCredentials !== undefined) setCorsAllowCredentials(cors.allowCredentials);
  }, [open, agent?.configurations?.corsConfig]);

  const hasWildcardOrigin = corsAllowAll || corsOrigins.includes("*");

  const { mutate: updateDeploySettings, isPending } = useUpdateAgentDeploySettings();

  const handleSave = useCallback(() => {
    updateDeploySettings(
      {
        params: { orgName, projName, agentName },
        body: {
          environmentName: environment,
          ...(agent?.configurations?.enableApiKeySecurity !== undefined && {
            enableApiKeySecurity: agent.configurations.enableApiKeySecurity,
          }),
          ...(agent?.configurations?.enableAutoInstrumentation !== undefined && {
            enableAutoInstrumentation: agent.configurations.enableAutoInstrumentation,
          }),
          ...(isApiAgent && {
            corsConfig: {
              enabled: corsEnabled,
              allowOrigin: hasWildcardOrigin ? ["*"] : corsOrigins,
              allowMethods: corsMethods,
              allowHeaders: corsHeaders,
              allowCredentials: hasWildcardOrigin ? false : corsAllowCredentials,
            },
          }),
        },
      },
      {
        onSuccess: () => onClose(),
        onError: (error) => {
          const body = (error as { body?: { message?: string } })?.body;
          pushSnackBar({ message: body?.message ?? "Failed to apply security configuration", type: "error" });
        },
      },
    );
  }, [
    environment, orgName, projName, agentName,
    agent?.configurations?.enableApiKeySecurity,
    agent?.configurations?.enableAutoInstrumentation,
    corsEnabled, corsOrigins, corsMethods, corsHeaders, corsAllowCredentials,
    hasWildcardOrigin, isApiAgent,
    updateDeploySettings, onClose, pushSnackBar,
  ]);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<Shield size={24} />} title="Update CORS Configuration" onClose={onClose} />
      <DrawerContent>
        <Form.Stack spacing={3}>

          {/* ── CORS ─────────────────────────────────────────────────── */}
          {isApiAgent && (
            <Form.Section>
              <Form.Header>CORS Configuration</Form.Header>
              <Form.Subheader>
                Control which origins, methods, and headers may access this endpoint.
              </Form.Subheader>
              <Form.Stack spacing={1}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={corsEnabled}
                      onChange={(_, checked) => setCorsEnabled(checked)}
                      disabled={isPending}
                    />
                  }
                  label="Enable CORS"
                />
                <Collapse in={corsEnabled}>
                  <Accordion
                    disableGutters
                    elevation={0}
                    sx={{ mt: 1, border: "1px solid", borderColor: "divider", borderRadius: 1, "&:before": { display: "none" } }}
                  >
                    <AccordionSummary expandIcon={<ChevronDown size={16} />}>
                      <Typography variant="body2">Advanced</Typography>
                    </AccordionSummary>
                    <AccordionDetails>
                      <Form.Stack spacing={2}>
                        <Box display="flex" gap={2} alignItems="center">
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={corsAllowAll}
                                onChange={(_, checked) => {
                                  setCorsAllowAll(checked);
                                  if (checked) {
                                    setCorsAllowCredentials(false);
                                    setCorsOrigins(["*"]);
                                  } else {
                                    setCorsOrigins((prev) => prev.filter((o) => o !== "*"));
                                  }
                                }}
                                disabled={isPending}
                              />
                            }
                            label="Allow all origins"
                          />
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={corsAllowCredentials}
                                onChange={(_, checked) => setCorsAllowCredentials(checked)}
                                disabled={isPending || hasWildcardOrigin}
                              />
                            }
                            label="Allow credentials"
                          />
                        </Box>
                        {!corsAllowAll && (
                          <FormControl fullWidth>
                            <FormLabel>Allowed origins</FormLabel>
                            <Autocomplete
                              multiple
                              freeSolo
                              options={[]}
                              value={corsOrigins}
                              onChange={(_, v) => setCorsOrigins(v as string[])}
                              renderTags={(vals, getTagProps) =>
                                vals.map((opt, i) => (
                                  <Chip label={opt as string} size="small" {...getTagProps({ index: i })} key={opt as string} />
                                ))
                              }
                              renderInput={(params) => (
                                <TextField {...params} size="small" placeholder="Add origin and press Enter" />
                              )}
                            />
                          </FormControl>
                        )}
                        <FormControl fullWidth>
                          <FormLabel>Allowed methods</FormLabel>
                          <Autocomplete
                            multiple
                            freeSolo
                            options={[]}
                            value={corsMethods}
                            onChange={(_, v) => setCorsMethods(v as string[])}
                            renderTags={(vals, getTagProps) =>
                              vals.map((opt, i) => (
                                <Chip label={opt as string} size="small" {...getTagProps({ index: i })} key={opt as string} />
                              ))
                            }
                            renderInput={(params) => (
                              <TextField {...params} size="small" placeholder="Add method and press Enter" />
                            )}
                          />
                        </FormControl>
                        <FormControl fullWidth>
                          <FormLabel>Allowed headers</FormLabel>
                          <Autocomplete
                            multiple
                            freeSolo
                            options={[]}
                            value={corsHeaders}
                            onChange={(_, v) => setCorsHeaders(v as string[])}
                            renderTags={(vals, getTagProps) =>
                              vals.map((opt, i) => (
                                <Chip label={opt as string} size="small" {...getTagProps({ index: i })} key={opt as string} />
                              ))
                            }
                            renderInput={(params) => (
                              <TextField {...params} size="small" placeholder="Add header and press Enter" />
                            )}
                          />
                        </FormControl>
                      </Form.Stack>
                    </AccordionDetails>
                  </Accordion>
                </Collapse>
              </Form.Stack>
            </Form.Section>
          )}

          <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
            <Button variant="outlined" onClick={onClose} disabled={isPending}>
              Cancel
            </Button>
            <Button
              variant="contained"
              color="primary"
              onClick={handleSave}
              disabled={isPending}
              startIcon={isPending ? <CircularProgress size={16} /> : undefined}
            >
              {isPending ? "Applying..." : "Apply"}
            </Button>
          </Box>
        </Form.Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}
