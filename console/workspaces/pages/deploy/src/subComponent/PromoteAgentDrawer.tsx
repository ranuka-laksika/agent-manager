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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Collapse,
  Divider,
  Form,
  FormControl,
  FormControlLabel,
  FormLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowUpFromLine, Plus } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  EnvVariableEditor,
} from "@agent-management-platform/views";
import {
  usePromoteAgent,
  useGetAgentConfigurations,
  useGetDeploymentPipeline,
} from "@agent-management-platform/api-client";
import type {
  Environment,
  EnvironmentVariable,
  FileMount,
  CorsConfig,
} from "@agent-management-platform/types";

interface PromoteAgentDrawerProps {
  open: boolean;
  onClose: () => void;
  sourceEnvironment: Environment;
  orgId: string;
  projectId: string;
  agentId: string;
}

interface PromoteFormState {
  targetEnvironment: string;
  useConfigFromSourceEnv: boolean;
  env: EnvironmentVariable[];
  files: FileMount[];
  enableAutoInstrumentation: boolean;
  enableApiKeySecurity: boolean;
  corsEnabled: boolean;
  corsAllowOrigin: string;
  corsAllowMethods: string;
  corsAllowHeaders: string;
  corsAllowCredentials: boolean;
}

const DEFAULT_STATE: PromoteFormState = {
  targetEnvironment: "",
  useConfigFromSourceEnv: true,
  env: [],
  files: [],
  enableAutoInstrumentation: false,
  enableApiKeySecurity: false,
  corsEnabled: false,
  corsAllowOrigin: "",
  corsAllowMethods: "",
  corsAllowHeaders: "",
  corsAllowCredentials: false,
};

export function PromoteAgentDrawer({
  open,
  onClose,
  sourceEnvironment,
  orgId,
  projectId,
  agentId,
}: PromoteAgentDrawerProps) {
  const [formState, setFormState] = useState<PromoteFormState>(DEFAULT_STATE);

  const { data: pipeline } = useGetDeploymentPipeline({ orgName: orgId, projName: projectId });
  const { data: sourceConfigs } = useGetAgentConfigurations(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { environment: sourceEnvironment.name },
  );

  const { mutateAsync: promoteAgent, isPending, error, reset: resetMutation } = usePromoteAgent();

  const targetEnvOptions = useMemo(() => {
    if (!pipeline) return [];
    const path = pipeline.promotionPaths.find(
      (p) => p.sourceEnvironmentRef === sourceEnvironment.name,
    );
    return path?.targetEnvironmentRefs ?? [];
  }, [pipeline, sourceEnvironment.name]);

  useEffect(() => {
    if (open) {
      setFormState({
        ...DEFAULT_STATE,
        targetEnvironment: targetEnvOptions[0]?.name ?? "",
      });
      resetMutation();
    }
  }, [open, resetMutation, targetEnvOptions]);

  const populateFromSource = useCallback(() => {
    if (!sourceConfigs) return;
    const cfg = sourceConfigs.configurations;
    setFormState((prev) => ({
      ...prev,
      env: cfg.env?.map((e) => ({ key: e.key, value: e.value, isSensitive: e.isSensitive })) ?? [],
      files: cfg.files ?? [],
    }));
  }, [sourceConfigs]);

  const handleToggleUseSourceConfig = useCallback(
    (checked: boolean) => {
      setFormState((prev) => ({ ...prev, useConfigFromSourceEnv: checked }));
      if (!checked) {
        populateFromSource();
      }
    },
    [populateFromSource],
  );

  const handleEnvChange = useCallback(
    (index: number, field: "key" | "value" | "isSensitive", value: string | boolean) => {
      setFormState((prev) => ({
        ...prev,
        env: prev.env.map((item, i) => (i === index ? { ...item, [field]: value } : item)),
      }));
    },
    [],
  );

  const handleAddEnv = useCallback(() => {
    setFormState((prev) => ({
      ...prev,
      env: [...prev.env, { key: "", value: "", isSensitive: false }],
    }));
  }, []);

  const handleRemoveEnv = useCallback((index: number) => {
    setFormState((prev) => ({
      ...prev,
      env: prev.env.filter((_, i) => i !== index),
    }));
  }, []);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!formState.targetEnvironment) return;

      const corsConfig: CorsConfig | undefined =
        formState.corsEnabled
          ? {
              enabled: true,
              allowOrigin: formState.corsAllowOrigin
                ? formState.corsAllowOrigin.split(",").map((s) => s.trim()).filter(Boolean)
                : undefined,
              allowMethods: formState.corsAllowMethods
                ? formState.corsAllowMethods.split(",").map((s) => s.trim()).filter(Boolean)
                : undefined,
              allowHeaders: formState.corsAllowHeaders
                ? formState.corsAllowHeaders.split(",").map((s) => s.trim()).filter(Boolean)
                : undefined,
              allowCredentials: formState.corsAllowCredentials,
            }
          : undefined;

      try {
        await promoteAgent({
          params: { orgName: orgId, projName: projectId, agentName: agentId },
          body: {
            sourceEnvironment: sourceEnvironment.name,
            targetEnvironment: formState.targetEnvironment,
            useConfigFromSourceEnv: formState.useConfigFromSourceEnv,
            ...(formState.useConfigFromSourceEnv
              ? {}
              : {
                  env: formState.env.filter((e) => e.key),
                  files: formState.files,
                  enableAutoInstrumentation: formState.enableAutoInstrumentation,
                  enableApiKeySecurity: formState.enableApiKeySecurity,
                  corsConfig,
                }),
          },
        });
        onClose();
      } catch {
        // handled by error
      }
    },
    [formState, promoteAgent, orgId, projectId, agentId, sourceEnvironment.name, onClose],
  );

  const errorMessage = useMemo(
    () => (error ? (error as Error)?.message ?? "Failed to promote agent" : null),
    [error],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<ArrowUpFromLine size={24} />}
        title="Promote Agent"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {errorMessage && (
              <Alert severity="error">
                <Typography variant="body2">{errorMessage}</Typography>
              </Alert>
            )}

            <Form.Section>
              <Form.Header>Promotion Details</Form.Header>
              <Form.Stack spacing={2}>
                <FormControl fullWidth>
                  <FormLabel>Source Environment</FormLabel>
                  <TextField
                    fullWidth
                    size="small"
                    value={sourceEnvironment.displayName ?? sourceEnvironment.name}
                    slotProps={{ input: { readOnly: true } }}
                    disabled
                  />
                </FormControl>

                <FormControl fullWidth required>
                  <FormLabel required>Target Environment</FormLabel>
                  <Select
                    size="small"
                    value={formState.targetEnvironment}
                    onChange={(e) =>
                      setFormState((prev) => ({
                        ...prev,
                        targetEnvironment: e.target.value as string,
                      }))
                    }
                    displayEmpty
                    disabled={isPending}
                  >
                    <MenuItem value="" disabled>
                      <em>Select target environment</em>
                    </MenuItem>
                    {targetEnvOptions.map((t) => (
                      <MenuItem key={t.name} value={t.name}>
                        {t.name}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Form.Stack>
            </Form.Section>

            <Divider />

            <Form.Section>
              <Form.Header>Configuration</Form.Header>
              <Form.Stack spacing={2}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formState.useConfigFromSourceEnv}
                      onChange={(e) => handleToggleUseSourceConfig(e.target.checked)}
                      disabled={isPending}
                    />
                  }
                  label={
                    <Stack>
                      <Typography variant="body2">Use config from source environment</Typography>
                      <Typography variant="caption" color="text.secondary">
                        Inherit environment variables and file mounts from{" "}
                        {sourceEnvironment.displayName ?? sourceEnvironment.name}
                      </Typography>
                    </Stack>
                  }
                />

                <Collapse in={!formState.useConfigFromSourceEnv} timeout="auto" unmountOnExit>
                  <Stack spacing={2}>
                    <Card variant="outlined">
                      <CardContent>
                        <Stack spacing={1.5}>
                          <Stack
                            direction="row"
                            justifyContent="space-between"
                            alignItems="center"
                          >
                            <Typography variant="h6">Environment Variables</Typography>
                            <Button
                              size="small"
                              variant="outlined"
                              startIcon={<Plus size={14} />}
                              onClick={handleAddEnv}
                              disabled={isPending}
                            >
                              Add
                            </Button>
                          </Stack>
                          {formState.env.length === 0 ? (
                            <Typography variant="body2" color="text.secondary">
                              No environment variables. Click Add to define them.
                            </Typography>
                          ) : (
                            <Stack spacing={1}>
                              {formState.env.map((item, index) => (
                                <EnvVariableEditor
                                  key={index}
                                  index={index}
                                  keyValue={item.key}
                                  valueValue={item.value}
                                  isSensitive={item.isSensitive ?? false}
                                  onKeyChange={(v) => handleEnvChange(index, "key", v)}
                                  onValueChange={(v) => handleEnvChange(index, "value", v)}
                                  onSensitiveChange={(v) =>
                                    handleEnvChange(index, "isSensitive", v)
                                  }
                                  onRemove={() => handleRemoveEnv(index)}
                                />
                              ))}
                            </Stack>
                          )}
                        </Stack>
                      </CardContent>
                    </Card>

                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={formState.enableAutoInstrumentation}
                          onChange={(e) =>
                            setFormState((prev) => ({
                              ...prev,
                              enableAutoInstrumentation: e.target.checked,
                            }))
                          }
                          disabled={isPending}
                        />
                      }
                      label="Enable auto instrumentation"
                    />

                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={formState.enableApiKeySecurity}
                          onChange={(e) =>
                            setFormState((prev) => ({
                              ...prev,
                              enableApiKeySecurity: e.target.checked,
                            }))
                          }
                          disabled={isPending}
                        />
                      }
                      label="Enable API key security"
                    />

                    <Card variant="outlined">
                      <CardContent>
                        <Stack spacing={1.5}>
                          <FormControlLabel
                            control={
                              <Switch
                                checked={formState.corsEnabled}
                                onChange={(e) =>
                                  setFormState((prev) => ({
                                    ...prev,
                                    corsEnabled: e.target.checked,
                                  }))
                                }
                                disabled={isPending}
                              />
                            }
                            label={<Typography variant="h6">CORS</Typography>}
                          />
                          <Collapse in={formState.corsEnabled} timeout="auto" unmountOnExit>
                            <Stack spacing={1.5}>
                              <FormControl fullWidth>
                                <FormLabel>Allow Origins</FormLabel>
                                <TextField
                                  size="small"
                                  value={formState.corsAllowOrigin}
                                  onChange={(e) =>
                                    setFormState((prev) => ({
                                      ...prev,
                                      corsAllowOrigin: e.target.value,
                                    }))
                                  }
                                  placeholder="e.g. https://example.com, https://app.example.com"
                                  helperText="Comma-separated list of allowed origins"
                                  disabled={isPending}
                                />
                              </FormControl>
                              <FormControl fullWidth>
                                <FormLabel>Allow Methods</FormLabel>
                                <TextField
                                  size="small"
                                  value={formState.corsAllowMethods}
                                  onChange={(e) =>
                                    setFormState((prev) => ({
                                      ...prev,
                                      corsAllowMethods: e.target.value,
                                    }))
                                  }
                                  placeholder="e.g. GET, POST, PUT"
                                  helperText="Comma-separated list of allowed HTTP methods"
                                  disabled={isPending}
                                />
                              </FormControl>
                              <FormControl fullWidth>
                                <FormLabel>Allow Headers</FormLabel>
                                <TextField
                                  size="small"
                                  value={formState.corsAllowHeaders}
                                  onChange={(e) =>
                                    setFormState((prev) => ({
                                      ...prev,
                                      corsAllowHeaders: e.target.value,
                                    }))
                                  }
                                  placeholder="e.g. Content-Type, Authorization"
                                  helperText="Comma-separated list of allowed headers"
                                  disabled={isPending}
                                />
                              </FormControl>
                              <FormControlLabel
                                control={
                                  <Checkbox
                                    checked={formState.corsAllowCredentials}
                                    onChange={(e) =>
                                      setFormState((prev) => ({
                                        ...prev,
                                        corsAllowCredentials: e.target.checked,
                                      }))
                                    }
                                    disabled={isPending}
                                  />
                                }
                                label="Allow credentials"
                              />
                            </Stack>
                          </Collapse>
                        </Stack>
                      </CardContent>
                    </Card>
                  </Stack>
                </Collapse>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button variant="outlined" color="inherit" onClick={onClose} disabled={isPending}>
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isPending || !formState.targetEnvironment}
              >
                {isPending ? "Promoting..." : "Promote"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
