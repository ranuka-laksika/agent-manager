/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useEffect, useState } from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, SlidersVertical } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  EnvVariableEditor,
  FileMountEditor,
  useSnackBar,
} from "@agent-management-platform/views";
import {
  useDeployAgent,
  useGetAgentConfigurations,
} from "@agent-management-platform/api-client";
import type { EnvironmentVariable, FileMount } from "@agent-management-platform/types";

export interface EditDeployConfigDrawerProps {
  open: boolean;
  onClose: () => void;
  imageId: string;
  orgName: string;
  projName: string;
  agentName: string;
  environment: string;
  title: string;
}

export function EditDeployConfigDrawer({
  open,
  onClose,
  imageId,
  orgName,
  projName,
  agentName,
  environment,
  title,
}: EditDeployConfigDrawerProps) {
  const { pushSnackBar } = useSnackBar();

  const { data: configurations } = useGetAgentConfigurations(
    { orgName, projName, agentName },
    { environment },
  );

  const [env, setEnv] = useState<EnvironmentVariable[]>([]);
  const [files, setFiles] = useState<FileMount[]>([]);

  useEffect(() => {
    if (!open) return;
    const cfg = configurations?.configurations;
    setEnv(cfg?.env?.map(
      (e) => ({ key: e.key, value: e.value, isSensitive: e.isSensitive }),
    ) ?? []);
    setFiles(cfg?.files ?? []);
  }, [open, configurations]);

  const { mutate: deployAgent, isPending } = useDeployAgent();

  const handleSave = useCallback(() => {
    const validEnv = env.filter((e) => e.key);
    const validFiles = files.filter((f) => f.key && f.mountPath);
    deployAgent(
      {
        params: { orgName, projName, agentName },
        body: {
          imageId,
          ...(validEnv.length && { env: validEnv }),
          ...(validFiles.length && { files: validFiles }),
        },
      },
      {
        onSuccess: () => onClose(),
        onError: (error) => {
          const body = (error as { body?: { message?: string } })?.body;
          pushSnackBar({ message: body?.message ?? "Failed to apply configuration", type: "error" });
        },
      },
    );
  }, [env, files, imageId, orgName, projName, agentName, deployAgent, onClose, pushSnackBar]);

  // ── Env handlers ─────────────────────────────────────────────────────────
  const handleAddEnv = useCallback(() => {
    setEnv((prev) => [...prev, { key: "", value: "", isSensitive: false }]);
  }, []);

  const handleEnvChange = useCallback(
    (index: number, field: "key" | "value" | "isSensitive", value: string | boolean) => {
      setEnv((prev) => prev.map((item, i) => (i === index ? { ...item, [field]: value } : item)));
    },
    [],
  );

  const handleRemoveEnv = useCallback((index: number) => {
    setEnv((prev) => prev.filter((_, i) => i !== index));
  }, []);

  // ── File handlers ─────────────────────────────────────────────────────────
  const handleAddFile = useCallback(() => {
    setFiles((prev) => [...prev, { key: "", mountPath: "", value: "" }]);
  }, []);

  const handleFileChange = useCallback(
    (index: number, field: "key" | "mountPath" | "value", value: string) => {
      setFiles((prev) => prev.map((f, i) => (i === index ? { ...f, [field]: value } : f)));
    },
    [],
  );

  const handleRemoveFile = useCallback((index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
  }, []);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<SlidersVertical size={24} />} title={title} onClose={onClose} />
      <DrawerContent>
        <Stack spacing={3}>
          <Card variant="outlined">
            <CardContent>
              <Stack spacing={1.5}>
                <Stack direction="row" justifyContent="space-between" alignItems="center">
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
                {env.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    No environment variables. Click Add to define them.
                  </Typography>
                ) : (
                  <Stack spacing={1}>
                    {env.map((item, index) => (
                      <EnvVariableEditor
                        key={index}
                        index={index}
                        keyValue={item.key}
                        valueValue={item.value}
                        isSensitive={item.isSensitive ?? false}
                        onKeyChange={(v) => handleEnvChange(index, "key", v)}
                        onValueChange={(v) => handleEnvChange(index, "value", v)}
                        onSensitiveChange={(v) => handleEnvChange(index, "isSensitive", v)}
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
                <Stack direction="row" justifyContent="space-between" alignItems="center">
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
                {files.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    No file mounts. Click Add to define them.
                  </Typography>
                ) : (
                  <Stack spacing={1}>
                    {files.map((file, index) => (
                      <FileMountEditor
                        key={index}
                        index={index}
                        keyValue={file.key}
                        mountPathValue={file.mountPath}
                        contentValue={file.value}
                        onKeyChange={(v) => handleFileChange(index, "key", v)}
                        onMountPathChange={(v) => handleFileChange(index, "mountPath", v)}
                        onContentChange={(v) => handleFileChange(index, "value", v)}
                        onRemove={() => handleRemoveFile(index)}
                      />
                    ))}
                  </Stack>
                )}
              </Stack>
            </CardContent>
          </Card>

          <Box display="flex" justifyContent="flex-end" gap={1}>
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
              {isPending ? "Applying..." : "Apply & Redeploy"}
            </Button>
          </Box>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}
