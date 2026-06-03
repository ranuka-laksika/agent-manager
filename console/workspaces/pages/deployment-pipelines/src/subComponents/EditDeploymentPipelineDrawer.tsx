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
  Form,
  FormControl,
  FormLabel,
  IconButton,
  MenuItem,
  Select,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowRight, Edit, Plus, X } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import {
  useUpdateDeploymentPipeline,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import type { DeploymentPipelineResponse } from "@agent-management-platform/types";
import { editPipelineSchema, type EditPipelineFormValues } from "../form/schema";
import { validatePromotionChain } from "../utils/validatePromotionChain";

interface EditDeploymentPipelineDrawerProps {
  open: boolean;
  onClose: () => void;
  pipeline: DeploymentPipelineResponse;
  orgId: string;
}

function pipelineToChain(pipeline: DeploymentPipelineResponse): string[] {
  if (pipeline.promotionPaths.length === 0) return ["", ""];
  const validation = validatePromotionChain(pipeline.promotionPaths);
  if (validation.valid && validation.chain && validation.chain.length >= 2) {
    return validation.chain;
  }
  // Fallback for invalid existing paths: collect sources + last target
  const sources = pipeline.promotionPaths.map((p) => p.sourceEnvironmentRef);
  const lastTarget = pipeline.promotionPaths[pipeline.promotionPaths.length - 1]?.targetEnvironmentRefs[0]?.name ?? "";
  return [...sources, lastTarget];
}

function chainToPromotionPaths(chain: string[]) {
  return chain.slice(0, -1).map((env, i) => ({
    sourceEnvironmentRef: env,
    targetEnvironmentRefs: [{ name: chain[i + 1] }],
  }));
}

export function EditDeploymentPipelineDrawer({
  open,
  onClose,
  pipeline,
  orgId,
}: EditDeploymentPipelineDrawerProps) {
  const [formData, setFormData] = useState<EditPipelineFormValues>(() => ({
    displayName: pipeline.displayName,
    description: pipeline.description ?? "",
    chain: pipelineToChain(pipeline),
  }));

  const [submitError, setSubmitError] = useState<string | null>(null);

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditPipelineFormValues>(editPipelineSchema);

  const {
    mutateAsync: updatePipeline,
    isPending: isUpdating,
    error: updateError,
    reset: resetMutation,
  } = useUpdateDeploymentPipeline();

  const { data: environments } = useListEnvironments({ orgName: orgId });
  const envOptions = useMemo(() => environments ?? [], [environments]);

  useEffect(() => {
    if (open) {
      setFormData({
        displayName: pipeline.displayName,
        description: pipeline.description ?? "",
        chain: pipelineToChain(pipeline),
      });
      setSubmitError(null);
      resetMutation();
    }
  }, [open, pipeline, resetMutation]);

  const handleFieldChange = useCallback(
    (field: "displayName" | "description", value: string) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value };
        const fieldError = validateField(field, next[field as keyof EditPipelineFormValues], next);
        setFieldError(field, fieldError);
        return next;
      });
    },
    [setFieldError, validateField],
  );

  const handleChainChange = useCallback((index: number, value: string) => {
    setFormData((prev) => {
      const chain = [...prev.chain];
      chain[index] = value;
      return { ...prev, chain };
    });
  }, []);

  const handleAddEnv = useCallback(() => {
    setFormData((prev) => ({ ...prev, chain: [...prev.chain, ""] }));
  }, []);

  const handleRemoveEnv = useCallback((index: number) => {
    setFormData((prev) => ({
      ...prev,
      chain: prev.chain.filter((_, i) => i !== index),
    }));
  }, []);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setSubmitError(null);

      const result = editPipelineSchema.safeParse(formData);
      if (!result.success) {
        validateForm(formData);
        return;
      }

      try {
        await updatePipeline({
          params: { orgName: orgId, projName: pipeline.name },
          body: {
            displayName: result.data.displayName.trim(),
            description: result.data.description?.trim(),
            promotionPaths: chainToPromotionPaths(result.data.chain),
          },
        });
        onClose();
      } catch {
        // handled by updateError
      }
    },
    [formData, validateForm, updatePipeline, orgId, pipeline.name, onClose],
  );

  const errorMessage = useMemo(() => {
    if (submitError) return submitError;
    if (updateError) return (updateError as Error)?.message ?? "Failed to update pipeline";
    return null;
  }, [submitError, updateError]);

  // Each selector only shows envs not already chosen elsewhere in the chain.
  const optionsFor = useCallback(
    (index: number) =>
      envOptions.filter(
        (e) => !formData.chain.some((v, i) => i !== index && v === e.name),
      ),
    [formData.chain, envOptions],
  );

  const canRemove = formData.chain.length > 2;
  const allFilled = formData.chain.every((v) => v !== "");

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<Edit size={24} />} title="Edit Deployment Pipeline" onClose={onClose} />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {errorMessage && (
              <Alert severity="error">
                <Typography variant="body2">{errorMessage}</Typography>
              </Alert>
            )}

            <Form.Section>
              <Form.Header>Pipeline Details</Form.Header>
              <Form.Stack spacing={2}>
                <FormControl fullWidth error={Boolean(errors.displayName)}>
                  <FormLabel required>Display Name</FormLabel>
                  <TextField
                    fullWidth
                    size="small"
                    value={formData.displayName}
                    onChange={(e) => handleFieldChange("displayName", e.target.value)}
                    placeholder="e.g., Production Pipeline"
                    error={Boolean(errors.displayName)}
                    helperText={errors.displayName}
                    disabled={isUpdating}
                  />
                </FormControl>
                <FormControl fullWidth>
                  <FormLabel>Description</FormLabel>
                  <TextField
                    fullWidth
                    size="small"
                    multiline
                    rows={2}
                    value={formData.description ?? ""}
                    onChange={(e) => handleFieldChange("description", e.target.value)}
                    placeholder="Optional description"
                    disabled={isUpdating}
                  />
                </FormControl>
              </Form.Stack>
            </Form.Section>

            <Form.Section>
              <Stack direction="row" justifyContent="space-between" alignItems="center" mb={1.5}>
                <Form.Header>Promotion Chain</Form.Header>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<Plus size={14} />}
                  onClick={handleAddEnv}
                  disabled={isUpdating || formData.chain.length >= envOptions.length}
                >
                  Add
                </Button>
              </Stack>

              {/* Scrollable chain row */}
              <Box sx={{ overflowX: "auto", pb: 1 }}>
                <Stack direction="row" alignItems="center" sx={{ minWidth: "max-content" }}>
                  {formData.chain.map((envName, index) => (
                    <Stack key={index} direction="row" alignItems="center">
                      {/* Env selector + remove */}
                      <Stack alignItems="center" spacing={0.5}>
                        <Stack direction="row" alignItems="center" spacing={0.5}>
                          <FormControl size="small" sx={{ minWidth: 120 }}>
                            <Select
                              size="small"
                              value={envName}
                              onChange={(e) => handleChainChange(index, e.target.value as string)}
                              disabled={isUpdating}
                              displayEmpty
                              renderValue={(v) => {
                                const label = v
                                  ? (envOptions.find((e) => e.name === v)?.displayName ?? v)
                                  : null;
                                return label ? (
                                  <Typography variant="body2">{label}</Typography>
                                ) : (
                                  <Typography variant="body2" color="text.disabled">
                                    Select
                                  </Typography>
                                );
                              }}
                            >
                              {optionsFor(index).map((env) => (
                                <MenuItem key={env.name} value={env.name}>
                                  {env.displayName ?? env.name}
                                </MenuItem>
                              ))}
                            </Select>
                          </FormControl>
                          {canRemove && (
                            <Tooltip title="Remove">
                              <IconButton
                                size="small"
                                onClick={() => handleRemoveEnv(index)}
                                disabled={isUpdating}
                              >
                                <X size={12} />
                              </IconButton>
                            </Tooltip>
                          )}
                        </Stack>
                      </Stack>

                      {/* Arrow between items */}
                      {index < formData.chain.length - 1 && (
                        <Box px={1} display="flex" alignItems="center">
                          <ArrowRight size={16} />
                        </Box>
                      )}
                    </Stack>
                  ))}
                </Stack>
              </Box>

              {!allFilled && (
                <Alert severity="warning" sx={{ mt: 1 }}>
                  <Typography variant="body2">Select an environment for each step.</Typography>
                </Alert>
              )}
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button variant="outlined" color="inherit" onClick={onClose} disabled={isUpdating}>
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isUpdating || !allFilled}
              >
                {isUpdating ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
