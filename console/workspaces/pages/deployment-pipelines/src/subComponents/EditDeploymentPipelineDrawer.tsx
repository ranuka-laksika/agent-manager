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
  Collapse,
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
import { Edit, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
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
import type {
  DeploymentPipelineResponse,
  PromotionPath,
} from "@agent-management-platform/types";
import { editPipelineSchema, type EditPipelineFormValues } from "../form/schema";

interface EditDeploymentPipelineDrawerProps {
  open: boolean;
  onClose: () => void;
  pipeline: DeploymentPipelineResponse;
  orgId: string;
}

function buildFormValues(pipeline: DeploymentPipelineResponse): EditPipelineFormValues {
  return {
    displayName: pipeline.displayName,
    description: pipeline.description ?? "",
    promotionPaths: pipeline.promotionPaths.map((p) => ({
      sourceEnvironmentRef: p.sourceEnvironmentRef,
      targetEnvironmentRefs: p.targetEnvironmentRefs.map((t) => ({ name: t.name })),
    })),
  };
}

export function EditDeploymentPipelineDrawer({
  open,
  onClose,
  pipeline,
  orgId,
}: EditDeploymentPipelineDrawerProps) {
  const [formData, setFormData] = useState<EditPipelineFormValues>(() =>
    buildFormValues(pipeline),
  );

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditPipelineFormValues>(editPipelineSchema);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<Partial<Record<string, string>>>({});

  const {
    mutateAsync: updatePipeline,
    isPending: isUpdating,
    error: updateError,
    reset: resetMutation,
  } = useUpdateDeploymentPipeline();

  const { data: environments } = useListEnvironments({ orgName: orgId });

  useEffect(() => {
    if (open) {
      setFormData(buildFormValues(pipeline));
      setLastSubmittedValidationErrors({});
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

  const handleAddPath = useCallback(() => {
    setFormData((prev) => ({
      ...prev,
      promotionPaths: [
        ...prev.promotionPaths,
        { sourceEnvironmentRef: "", targetEnvironmentRefs: [] },
      ],
    }));
  }, []);

  const handleRemovePath = useCallback((index: number) => {
    setFormData((prev) => ({
      ...prev,
      promotionPaths: prev.promotionPaths.filter((_, i) => i !== index),
    }));
  }, []);

  const handlePathSourceChange = useCallback((index: number, value: string) => {
    setFormData((prev) => {
      const paths = [...prev.promotionPaths];
      paths[index] = { ...paths[index], sourceEnvironmentRef: value };
      return { ...prev, promotionPaths: paths };
    });
  }, []);

  const handlePathTargetsChange = useCallback((index: number, values: string[]) => {
    setFormData((prev) => {
      const paths = [...prev.promotionPaths];
      paths[index] = {
        ...paths[index],
        targetEnvironmentRefs: values.map((name) => ({ name })),
      };
      return { ...prev, promotionPaths: paths };
    });
  }, []);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const result = editPipelineSchema.safeParse(formData);
      if (!result.success) {
        const fieldErrors: Partial<Record<string, string>> = {};
        result.error.issues.forEach((issue) => {
          const key = issue.path.join(".");
          if (!fieldErrors[key]) {
            fieldErrors[key] = issue.message;
          }
        });
        setLastSubmittedValidationErrors(fieldErrors);
        validateForm(formData);
        return;
      }
      setLastSubmittedValidationErrors({});

      const promotionPaths: PromotionPath[] = result.data.promotionPaths.map((p) => ({
        sourceEnvironmentRef: p.sourceEnvironmentRef,
        targetEnvironmentRefs: p.targetEnvironmentRefs,
      }));

      try {
        await updatePipeline({
          params: { orgName: orgId, projName: pipeline.name },
          body: {
            displayName: result.data.displayName.trim(),
            description: result.data.description?.trim(),
            promotionPaths,
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
    if (updateError) {
      return (updateError as Error)?.message ?? "Failed to update pipeline";
    }
    return null;
  }, [updateError]);

  const validationErrorsList = Object.values(lastSubmittedValidationErrors).filter(Boolean);

  const envOptions = environments ?? [];

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

            <Collapse in={validationErrorsList.length > 0} timeout="auto" unmountOnExit>
              <Alert severity="error" sx={{ mb: 2 }}>
                {validationErrorsList.map((error, index) => (
                  <Box key={index}>{error}</Box>
                ))}
              </Alert>
            </Collapse>

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
              <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Form.Header>Promotion Paths</Form.Header>
                <Tooltip title="Add promotion path">
                  <IconButton size="small" onClick={handleAddPath} disabled={isUpdating}>
                    <Plus size={16} />
                  </IconButton>
                </Tooltip>
              </Stack>
              <Form.Stack spacing={2}>
                {formData.promotionPaths.map((path, index) => (
                  <Box
                    key={index}
                    sx={{ border: "1px solid", borderColor: "divider", borderRadius: 1, p: 2 }}
                  >
                    <Stack spacing={1.5}>
                      <Stack direction="row" justifyContent="space-between" alignItems="center">
                        <Typography variant="body2" fontWeight="medium">
                          Path {index + 1}
                        </Typography>
                        <Tooltip title="Remove path">
                          <IconButton
                            size="small"
                            onClick={() => handleRemovePath(index)}
                            disabled={isUpdating || formData.promotionPaths.length <= 1}
                          >
                            <Trash size={14} />
                          </IconButton>
                        </Tooltip>
                      </Stack>
                      <FormControl fullWidth size="small">
                        <FormLabel required>Source Environment</FormLabel>
                        <Select
                          size="small"
                          value={path.sourceEnvironmentRef}
                          onChange={(e) => handlePathSourceChange(index, e.target.value as string)}
                          disabled={isUpdating}
                          displayEmpty
                        >
                          <MenuItem value="" disabled>
                            <em>Select source environment</em>
                          </MenuItem>
                          {envOptions.map((env) => (
                            <MenuItem key={env.name} value={env.name}>
                              {env.displayName ?? env.name}
                            </MenuItem>
                          ))}
                        </Select>
                      </FormControl>
                      <FormControl fullWidth size="small">
                        <FormLabel required>Target Environments</FormLabel>
                        <Select
                          size="small"
                          multiple
                          value={path.targetEnvironmentRefs.map((t) => t.name)}
                          onChange={(e) =>
                            handlePathTargetsChange(index, e.target.value as string[])
                          }
                          disabled={isUpdating}
                          displayEmpty
                          renderValue={(selected) => {
                            const arr = selected as string[];
                            if (arr.length === 0) return <em>Select target environments</em>;
                            return arr
                              .map(
                                (name) =>
                                  envOptions.find((e) => e.name === name)?.displayName ?? name,
                              )
                              .join(", ");
                          }}
                        >
                          {envOptions
                            .filter((env) => env.name !== path.sourceEnvironmentRef)
                            .map((env) => (
                              <MenuItem key={env.name} value={env.name}>
                                {env.displayName ?? env.name}
                              </MenuItem>
                            ))}
                        </Select>
                      </FormControl>
                    </Stack>
                  </Box>
                ))}
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button variant="outlined" color="inherit" onClick={onClose} disabled={isUpdating}>
                Cancel
              </Button>
              <Button type="submit" variant="contained" color="primary" disabled={isUpdating}>
                {isUpdating ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
