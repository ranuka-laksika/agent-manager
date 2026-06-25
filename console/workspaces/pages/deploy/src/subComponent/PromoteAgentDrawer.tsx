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
  Collapse,
  Divider,
  Form,
  FormControl,
  FormControlLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowUpFromLine, Plus } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  EnvVariableEditor,
  FileMountEditor,
} from "@agent-management-platform/views";
import {
  usePromoteAgent,
  useGetAgentConfigurations,
  useGetDeploymentPipeline,
  useListAgentDeployments,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import type {
  Environment,
  EnvironmentVariable,
  FileMount,
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
}

const DEFAULT_STATE: PromoteFormState = {
  targetEnvironment: "",
  useConfigFromSourceEnv: false,
  env: [],
  files: [],
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

  const { data: pipeline } = useGetDeploymentPipeline({
    orgName: orgId,
    projName: projectId,
  });
  const { data: environments } = useListEnvironments({ orgName: orgId });

  const envDisplayName = useCallback(
    (name: string) =>
      environments?.find((e) => e.name === name)?.displayName ?? name,
    [environments],
  );

  const {
    mutateAsync: promoteAgent,
    isPending,
    error,
    reset: resetMutation,
  } = usePromoteAgent();

  const targetEnvOptions = useMemo(() => {
    if (!pipeline) return [];
    const path = pipeline.promotionPaths.find(
      (p) => p.sourceEnvironmentRef === sourceEnvironment.name,
    );
    return path?.targetEnvironmentRefs ?? [];
  }, [pipeline, sourceEnvironment.name]);

  // Existing configuration of the selected destination environment. Keyed on the
  // target env, so selecting a different target refetches that env's config.
  const { data: targetConfigs, isSuccess: targetConfigLoaded } =
    useGetAgentConfigurations(
      { orgName: orgId, projName: projectId, agentName: agentId },
      { environment: formState.targetEnvironment },
    );

  // Deployment status per environment, used only to tell whether the chosen
  // target has ever been deployed. Drives the wording of the config-source
  // hint below (base config on a first promotion vs the target's own current
  // config on a re-promotion).
  const { data: deployments } = useListAgentDeployments({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const targetAlreadyDeployed = useMemo(() => {
    const status = deployments?.[formState.targetEnvironment]?.status;
    return !!status && status !== "not-deployed";
  }, [deployments, formState.targetEnvironment]);

  // Tracks which target env we've already pre-filled the editor for, so we fill
  // once per target rather than on every background refetch.
  const [filledForTarget, setFilledForTarget] = useState<string | null>(null);

  // Pick a default target environment when the drawer opens, and clear state on close.
  useEffect(() => {
    if (!open) {
      setFilledForTarget(null);
      setFormState(DEFAULT_STATE);
      resetMutation();
      return;
    }
    setFormState((prev) =>
      prev.targetEnvironment
        ? prev
        : { ...prev, targetEnvironment: targetEnvOptions[0]?.name ?? "" },
    );
  }, [open, targetEnvOptions, resetMutation]);

  // Pre-fill the editor with the destination environment's existing config so the
  // user edits from its previous values rather than starting blank. Only the
  // user-managed keys (isSystem=false) are editable; system entries are
  // platform-injected. We wait for the target's query to settle (targetConfigLoaded)
  // before filling, so switching to a target with no config clears the previous
  // target's values to empty rather than leaving them stale. We fill once per
  // target (tracked by filledForTarget) so a background refetch of the same target
  // doesn't clobber in-progress edits.
  useEffect(() => {
    if (!open) return;
    const target = formState.targetEnvironment;
    if (!target || filledForTarget === target || !targetConfigLoaded) return;
    const cfg = targetConfigs?.configurations;
    const userEditableEnv = (cfg?.env ?? [])
      .filter((e) => !e.isSystem)
      .map((e) => ({
        key: e.key,
        value: e.value ?? "",
        isSensitive: e.isSensitive,
        secretRef: e.secretRef,
      }));
    setFormState((prev) => ({
      ...prev,
      env: userEditableEnv,
      files: cfg?.files ?? [],
    }));
    setFilledForTarget(target);
  }, [
    open,
    formState.targetEnvironment,
    targetConfigLoaded,
    targetConfigs,
    filledForTarget,
  ]);

  const handleToggleUseSourceConfig = useCallback((checked: boolean) => {
    setFormState((prev) => ({ ...prev, useConfigFromSourceEnv: checked }));
  }, []);

  // secretRef is intentionally preserved while editing so cancelling an edit can
  // restore the original masked secret. Submit decides whether to send the new
  // value or fall back to secretRef (see handleSubmit).
  const handleEnvChange = useCallback(
    (
      index: number,
      field: "key" | "value" | "isSensitive",
      value: string | boolean,
    ) => {
      setFormState((prev) => ({
        ...prev,
        env: prev.env.map((item, i) =>
          i === index ? { ...item, [field]: value } : item,
        ),
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

  const handleAddFile = useCallback(() => {
    setFormState((prev) => ({
      ...prev,
      files: [...prev.files, { key: "", mountPath: "", value: "" }],
    }));
  }, []);

  const handleFileChange = useCallback(
    (index: number, field: "key" | "mountPath" | "value", value: string) => {
      setFormState((prev) => ({
        ...prev,
        files: prev.files.map((f, i) =>
          i === index ? { ...f, [field]: value } : f,
        ),
      }));
    },
    [],
  );

  const handleRemoveFile = useCallback((index: number) => {
    setFormState((prev) => ({
      ...prev,
      files: prev.files.filter((_, i) => i !== index),
    }));
  }, []);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!formState.targetEnvironment) return;

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
                  env: formState.env
                    .filter((envVar) => envVar.key)
                    .map(({ key, value, isSensitive, secretRef }) =>
                      // Preserve the secret reference for secrets the user did not edit.
                      isSensitive && secretRef && !value
                        ? ({
                            key,
                            isSensitive,
                            secretRef,
                          } as EnvironmentVariable)
                        : { key, value, isSensitive },
                    ),
                  files: formState.files,
                }),
          },
        });
        onClose();
      } catch {
        // handled by error
      }
    },
    [
      formState,
      promoteAgent,
      orgId,
      projectId,
      agentId,
      sourceEnvironment.name,
      onClose,
    ],
  );

  const errorMessage = useMemo(
    () =>
      error ? ((error as Error)?.message ?? "Failed to promote agent") : null,
    [error],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<ArrowUpFromLine size={24} />}
        title={`Promote from ${sourceEnvironment.displayName ?? sourceEnvironment.name} Environment`}
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

            {targetEnvOptions.length > 1 && (
              <>
                <Form.Section>
                  <Form.Header>Target Environment</Form.Header>
                  <Form.Stack spacing={2}>
                    <FormControl fullWidth required>
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
                            {envDisplayName(t.name)}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                  </Form.Stack>
                </Form.Section>

                <Divider />
              </>
            )}

            <Form.Section>
              <Form.Header>Configuration</Form.Header>
              <Form.Stack spacing={2}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formState.useConfigFromSourceEnv}
                      onChange={(e) =>
                        handleToggleUseSourceConfig(e.target.checked)
                      }
                      disabled={isPending}
                    />
                  }
                  label={
                    <Stack>
                      <Typography variant="body2">
                        Use config from source environment
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        Inherit environment variables and file mounts from{" "}
                        {sourceEnvironment.displayName ??
                          sourceEnvironment.name}
                      </Typography>
                    </Stack>
                  }
                />

                <Collapse
                  in={!formState.useConfigFromSourceEnv}
                  timeout="auto"
                  unmountOnExit
                >
                  <Stack spacing={2}>
                    <Alert severity="info">
                      <Typography variant="body2">
                        {targetAlreadyDeployed
                          ? `These values are the current configuration of ${envDisplayName(
                              formState.targetEnvironment,
                            )}. Editing them here will update that environment on promote.`
                          : `These values are inherited from the agent's base configuration. Review and adjust them for ${envDisplayName(
                              formState.targetEnvironment,
                            )} before promoting.`}
                      </Typography>
                    </Alert>

                    <Card variant="outlined">
                      <CardContent>
                        <Stack spacing={1.5}>
                          <Stack
                            direction="row"
                            justifyContent="space-between"
                            alignItems="center"
                          >
                            <Typography variant="h6">
                              Environment Variables
                            </Typography>
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
                              No environment variables. Click Add to define
                              them.
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
                                  isExistingSecret={
                                    !!(item.secretRef && item.isSensitive)
                                  }
                                  onKeyChange={(v) =>
                                    handleEnvChange(index, "key", v)
                                  }
                                  onValueChange={(v) =>
                                    handleEnvChange(index, "value", v)
                                  }
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

                    <Card variant="outlined">
                      <CardContent>
                        <Stack spacing={1.5}>
                          <Stack
                            direction="row"
                            justifyContent="space-between"
                            alignItems="center"
                          >
                            <Typography variant="h6">File Mounts</Typography>
                            <Button
                              size="small"
                              variant="outlined"
                              startIcon={<Plus size={14} />}
                              onClick={handleAddFile}
                              disabled={isPending}
                            >
                              Add
                            </Button>
                          </Stack>
                          {formState.files.length === 0 ? (
                            <Typography variant="body2" color="text.secondary">
                              No file mounts. Click Add to define them.
                            </Typography>
                          ) : (
                            <Stack spacing={1}>
                              {formState.files.map((file, index) => (
                                <FileMountEditor
                                  key={index}
                                  index={index}
                                  keyValue={file.key}
                                  mountPathValue={file.mountPath}
                                  contentValue={file.value}
                                  onKeyChange={(v) =>
                                    handleFileChange(index, "key", v)
                                  }
                                  onMountPathChange={(v) =>
                                    handleFileChange(index, "mountPath", v)
                                  }
                                  onContentChange={(v) =>
                                    handleFileChange(index, "value", v)
                                  }
                                  onRemove={() => handleRemoveFile(index)}
                                />
                              ))}
                            </Stack>
                          )}
                        </Stack>
                      </CardContent>
                    </Card>
                  </Stack>
                </Collapse>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isPending}
              >
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
