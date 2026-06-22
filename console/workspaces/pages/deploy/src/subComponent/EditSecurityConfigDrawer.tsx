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
  useListEnvironmentIdentityProviders,
  useUpdateAgentDeploySettings,
} from "@agent-management-platform/api-client";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
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
  FormHelperText,
  FormLabel,
  Radio,
  RadioGroup,
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
import { useCallback, useEffect, useMemo, useState } from "react";

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
  useGetAgentConfigurations({ orgName, projName, agentName }, { environment });

  const isApiAgent = agent?.agentType?.type === "agent-api";

  // Issuer options are the environment's configured identity providers — agents
  // can only reference providers that exist on the environment's gateway.
  const { data: idpResp } = useListEnvironmentIdentityProviders({
    orgName,
    environmentId: open && isApiAgent ? environment : undefined,
  });
  const identityProviderOptions = useMemo(
    () => (idpResp?.list ?? []).map((p) => p.name),
    [idpResp],
  );
  const hasIdentityProviders = identityProviderOptions.length > 0;

  // ── State ─────────────────────────────────────────────────────────────────
  // Endpoint authentication mode — mutually exclusive by construction.
  // Maps to two backend booleans: apikey -> enableApiKeySecurity, oauth -> enableOAuthSecurity.
  const [authMode, setAuthMode] = useState<"none" | "apikey" | "oauth">(
    "apikey",
  );
  const [oauthIssuers, setOauthIssuers] = useState<string[]>([]);
  const [oauthAudiences, setOauthAudiences] = useState<string[]>([]);
  const [oauthHeaderName, setOauthHeaderName] =
    useState<string>("Authorization");
  const [oauthHeaderPrefix, setOauthHeaderPrefix] = useState<string>("Bearer");
  const [oauthForwardToken, setOauthForwardToken] = useState<boolean>(true);

  const [corsEnabled, setCorsEnabled] = useState(false);
  const [corsAllowAll, setCorsAllowAll] = useState(true);
  const [corsOrigins, setCorsOrigins] = useState<string[]>(["*"]);
  const [corsMethods, setCorsMethods] = useState<string[]>([
    "GET",
    "POST",
    "PUT",
    "DELETE",
    "PATCH",
    "OPTIONS",
  ]);
  const [corsHeaders, setCorsHeaders] = useState<string[]>([
    "authorization",
    "Content-Type",
    "Origin",
    "X-API-Key",
  ]);
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
      const isWildcard =
        cors.allowOrigin.length === 1 && cors.allowOrigin[0] === "*";
      setCorsAllowAll(isWildcard);
      setCorsOrigins(cors.allowOrigin);
    }
    if (cors.allowMethods !== undefined) setCorsMethods(cors.allowMethods);
    if (cors.allowHeaders !== undefined) setCorsHeaders(cors.allowHeaders);
    if (cors.allowCredentials !== undefined)
      setCorsAllowCredentials(cors.allowCredentials);
  }, [open, agent?.configurations?.corsConfig]);

  // Seed Endpoint Authentication form from agent config when drawer opens.
  useEffect(() => {
    if (!open) return;
    const cfg = agent?.configurations;
    if (cfg?.enableOAuthSecurity) {
      setAuthMode("oauth");
    } else if (cfg?.enableApiKeySecurity) {
      setAuthMode("apikey");
    } else if (cfg?.enableApiKeySecurity === false) {
      setAuthMode("none");
    } else {
      setAuthMode("apikey");
    }

    const oauth = cfg?.oauthConfig;
    setOauthIssuers(oauth?.issuers ?? []);
    setOauthAudiences(oauth?.audiences ?? []);
    setOauthHeaderName(oauth?.headerName || "Authorization");
    setOauthHeaderPrefix(oauth?.authHeaderPrefix || "Bearer");
    setOauthForwardToken(oauth?.forwardToken ?? true);
  }, [
    open,
    agent?.configurations?.enableApiKeySecurity,
    agent?.configurations?.enableOAuthSecurity,
    agent?.configurations?.oauthConfig,
  ]);

  const hasWildcardOrigin = corsAllowAll || corsOrigins.includes("*");

  // OAuth requires at least one identity provider (issuer). Block Apply otherwise.
  const oauthInvalid =
    isApiAgent && authMode === "oauth" && oauthIssuers.length === 0;

  const { mutate: updateDeploySettings, isPending } =
    useUpdateAgentDeploySettings();

  const handleSave = useCallback(() => {
    updateDeploySettings(
      {
        params: { orgName, projName, agentName },
        body: {
          environmentName: environment,
          ...(isApiAgent && {
            enableApiKeySecurity: authMode === "apikey",
            enableOAuthSecurity: authMode === "oauth",
          }),
          ...(isApiAgent &&
            authMode === "oauth" && {
              oauthConfig: {
                issuers: oauthIssuers,
                audiences: oauthAudiences,
                headerName: oauthHeaderName.trim() || "Authorization",
                authHeaderPrefix: oauthHeaderPrefix.trim() || "Bearer",
                forwardToken: oauthForwardToken,
              },
            }),
          ...(agent?.configurations?.enableAutoInstrumentation !==
            undefined && {
            enableAutoInstrumentation:
              agent.configurations.enableAutoInstrumentation,
          }),
          ...(isApiAgent && {
            corsConfig: {
              enabled: corsEnabled,
              allowOrigin: hasWildcardOrigin ? ["*"] : corsOrigins,
              allowMethods: corsMethods,
              allowHeaders: corsHeaders,
              allowCredentials: hasWildcardOrigin
                ? false
                : corsAllowCredentials,
            },
          }),
        },
      },
      {
        onSuccess: () => onClose(),
        onError: (error) => {
          const body = (error as { body?: { message?: string } })?.body;
          pushSnackBar({
            message: body?.message ?? "Failed to apply security configuration",
            type: "error",
          });
        },
      },
    );
  }, [
    environment,
    orgName,
    projName,
    agentName,
    agent?.configurations?.enableAutoInstrumentation,
    authMode,
    oauthIssuers,
    oauthAudiences,
    oauthHeaderName,
    oauthHeaderPrefix,
    oauthForwardToken,
    corsEnabled,
    corsOrigins,
    corsMethods,
    corsHeaders,
    corsAllowCredentials,
    hasWildcardOrigin,
    isApiAgent,
    updateDeploySettings,
    onClose,
    pushSnackBar,
  ]);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Shield size={24} />}
        title="Update Configurations"
        onClose={onClose}
      />
      <DrawerContent>
        <Form.Stack spacing={3}>
          {/* ── Endpoint Authentication ──────────────────────────────── */}
          {isApiAgent && (
            <Form.Section>
              <Form.Header>Endpoint Authentication</Form.Header>
              <Form.Subheader>
                Choose how callers authenticate against this agent endpoint.
              </Form.Subheader>
              <Form.Stack spacing={1}>
                <FormControl>
                  <RadioGroup
                    value={authMode}
                    onChange={(_, value) => {
                      const mode = value as "none" | "apikey" | "oauth";
                      setAuthMode(mode);
                      // Seed issuers from the environment's identity providers when
                      // switching into OAuth with none selected yet.
                      if (
                        mode === "oauth" &&
                        oauthIssuers.length === 0 &&
                        hasIdentityProviders
                      ) {
                        setOauthIssuers(identityProviderOptions);
                      }
                    }}
                  >
                    <FormControlLabel
                      value="none"
                      control={<Radio disabled={isPending} />}
                      label="No authentication"
                    />
                    <FormControlLabel
                      value="apikey"
                      control={<Radio disabled={isPending} />}
                      label="API key"
                    />
                    <FormControlLabel
                      value="oauth"
                      control={
                        <Radio disabled={isPending || !hasIdentityProviders} />
                      }
                      label={
                        hasIdentityProviders
                          ? "OAuth"
                          : "OAuth — configure an identity provider first"
                      }
                    />
                  </RadioGroup>
                </FormControl>

                <Collapse in={authMode === "apikey"}>
                  <FormControl fullWidth sx={{ mt: 1 }}>
                    <FormLabel>Header</FormLabel>
                    <TextField
                      value="X-API-Key"
                      size="small"
                      fullWidth
                      disabled
                    />
                  </FormControl>
                </Collapse>

                <Collapse in={authMode === "oauth"}>
                  <Form.Stack spacing={2} sx={{ mt: 1 }}>
                    {!hasIdentityProviders && (
                      <Alert severity="warning">
                        <Typography variant="caption">
                          No identity providers for this environment. Add one
                          under Security &rarr; Identity Providers first.
                        </Typography>
                      </Alert>
                    )}

                    <FormControl fullWidth error={oauthInvalid}>
                      <FormLabel required>Identity Providers</FormLabel>
                      <Autocomplete
                        multiple
                        options={identityProviderOptions}
                        value={oauthIssuers}
                        onChange={(_, v) => setOauthIssuers(v as string[])}
                        disabled={isPending || !hasIdentityProviders}
                        renderTags={(vals, getTagProps) =>
                          vals.map((opt, i) => (
                            <Chip
                              label={opt as string}
                              size="small"
                              {...getTagProps({ index: i })}
                              key={opt as string}
                            />
                          ))
                        }
                        renderInput={(params) => (
                          <TextField
                            {...params}
                            size="small"
                            error={oauthInvalid}
                            placeholder={
                              oauthIssuers.length === 0
                                ? "Select identity providers"
                                : ""
                            }
                          />
                        )}
                      />
                      {oauthInvalid && (
                        <FormHelperText>
                          Select at least one identity provider.
                        </FormHelperText>
                      )}
                    </FormControl>

                    <FormControl fullWidth>
                      <FormLabel>Audiences</FormLabel>
                      <Autocomplete
                        multiple
                        freeSolo
                        options={[]}
                        value={oauthAudiences}
                        onChange={(_, v) => setOauthAudiences(v as string[])}
                        disabled={isPending}
                        renderTags={(vals, getTagProps) =>
                          vals.map((opt, i) => (
                            <Chip
                              label={opt as string}
                              size="small"
                              {...getTagProps({ index: i })}
                              key={opt as string}
                            />
                          ))
                        }
                        renderInput={(params) => (
                          <TextField
                            {...params}
                            size="small"
                            placeholder={
                              oauthAudiences.length === 0
                                ? "Add accepted audience (aud) values"
                                : ""
                            }
                          />
                        )}
                      />
                      <FormHelperText>
                        Accepted token audiences (aud claim). Leave empty to
                        disable audience validation.
                      </FormHelperText>
                    </FormControl>

                    <Box display="flex" gap={2}>
                      <FormControl fullWidth>
                        <FormLabel>Header name</FormLabel>
                        <TextField
                          size="small"
                          fullWidth
                          value={oauthHeaderName}
                          disabled={isPending}
                          onChange={(e) => setOauthHeaderName(e.target.value)}
                          placeholder="Authorization"
                        />
                      </FormControl>
                      <FormControl fullWidth>
                        <FormLabel>Auth header prefix</FormLabel>
                        <TextField
                          size="small"
                          fullWidth
                          value={oauthHeaderPrefix}
                          disabled={isPending}
                          onChange={(e) => setOauthHeaderPrefix(e.target.value)}
                          placeholder="Bearer"
                        />
                      </FormControl>
                    </Box>

                    <FormControlLabel
                      control={
                        <Switch
                          checked={oauthForwardToken}
                          onChange={(_, checked) =>
                            setOauthForwardToken(checked)
                          }
                          disabled={isPending}
                        />
                      }
                      label="Forward token to upstream"
                    />
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ mt: -1 }}
                    >
                      Forward the token header to the upstream service after
                      validation. Disable to strip it before proxying.
                    </Typography>
                  </Form.Stack>
                </Collapse>
              </Form.Stack>
            </Form.Section>
          )}

          {/* ── CORS ─────────────────────────────────────────────────── */}
          {isApiAgent && (
            <Form.Section>
              <Form.Header>CORS Configuration</Form.Header>
              <Form.Subheader>
                Control which origins, methods, and headers may access this
                endpoint.
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
                    sx={{
                      mt: 1,
                      border: "1px solid",
                      borderColor: "divider",
                      borderRadius: 1,
                      "&:before": { display: "none" },
                    }}
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
                                    setCorsOrigins((prev) =>
                                      prev.filter((o) => o !== "*"),
                                    );
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
                                onChange={(_, checked) =>
                                  setCorsAllowCredentials(checked)
                                }
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
                                  <Chip
                                    label={opt as string}
                                    size="small"
                                    {...getTagProps({ index: i })}
                                    key={opt as string}
                                  />
                                ))
                              }
                              renderInput={(params) => (
                                <TextField
                                  {...params}
                                  size="small"
                                  placeholder="Add origin and press Enter"
                                />
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
                                <Chip
                                  label={opt as string}
                                  size="small"
                                  {...getTagProps({ index: i })}
                                  key={opt as string}
                                />
                              ))
                            }
                            renderInput={(params) => (
                              <TextField
                                {...params}
                                size="small"
                                placeholder="Add method and press Enter"
                              />
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
                                <Chip
                                  label={opt as string}
                                  size="small"
                                  {...getTagProps({ index: i })}
                                  key={opt as string}
                                />
                              ))
                            }
                            renderInput={(params) => (
                              <TextField
                                {...params}
                                size="small"
                                placeholder="Add header and press Enter"
                              />
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
              disabled={isPending || oauthInvalid}
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
