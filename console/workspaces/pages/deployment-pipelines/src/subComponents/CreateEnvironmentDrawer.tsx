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
  Checkbox,
  Form,
  FormControl,
  FormControlLabel,
  FormLabel,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import {
  useCreateEnvironment,
  useListDataPlanes,
} from "@agent-management-platform/api-client";
import type { DataPlane } from "@agent-management-platform/types";
import { createEnvironmentSchema, type CreateEnvironmentFormValues } from "../form/environmentSchema";

interface CreateEnvironmentDrawerProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
}

const DEFAULT_FORM: CreateEnvironmentFormValues = {
  name: "",
  displayName: "",
  description: "",
  dataplaneRef: "",
  dnsPrefix: "",
  isProduction: false,
};

// Converts a display name to a valid lowercase-alphanumeric-hyphen name.
function deriveNameFromDisplayName(displayName: string): string {
  return displayName
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-]/g, "")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

export function CreateEnvironmentDrawer({ open, onClose, orgId }: CreateEnvironmentDrawerProps) {
  const [formData, setFormData] = useState<CreateEnvironmentFormValues>(DEFAULT_FORM);

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreateEnvironmentFormValues>(createEnvironmentSchema);

  const {
    mutateAsync: createEnvironment,
    isPending,
    error: createError,
    reset: resetMutation,
  } = useCreateEnvironment();

  const { data: dataPlanes } = useListDataPlanes({ orgName: orgId });
  const planes = useMemo(() => dataPlanes ?? [], [dataPlanes]);

  useEffect(() => {
    if (open) {
      setFormData(DEFAULT_FORM);
      resetMutation();
    }
  }, [open, resetMutation]);

  // Auto-select first data plane when loaded.
  useEffect(() => {
    if (!formData.dataplaneRef && planes.length > 0) {
      setFormData((prev) => ({ ...prev, dataplaneRef: planes[0].name }));
    }
  }, [planes, formData.dataplaneRef]);

  const handleChange = useCallback(
    (field: keyof CreateEnvironmentFormValues, value: string | boolean) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value } as CreateEnvironmentFormValues;
        const err = validateField(field, next[field], next);
        setFieldError(field, err);
        return next;
      });
    },
    [validateField, setFieldError],
  );

  // Auto-derive name from displayName, and dnsPrefix from name, unless manually edited.
  const handleDisplayNameChange = useCallback(
    (value: string) => {
      setFormData((prev) => {
        const derivedName = deriveNameFromDisplayName(value);
        const nameInSync =
          prev.name === "" || prev.name === deriveNameFromDisplayName(prev.displayName);
        const newName = nameInSync ? derivedName : prev.name;
        const next = {
          ...prev,
          displayName: value,
          name: newName,
          dnsPrefix: newName,
        };
        setFieldError("displayName", validateField("displayName", value, next));
        setFieldError("name", validateField("name", newName, next));
        return next;
      });
    },
    [validateField, setFieldError],
  );

  // dnsPrefix always tracks name — no separate user input.
  const handleNameChange = useCallback(
    (value: string) => {
      setFormData((prev) => {
        const next = { ...prev, name: value, dnsPrefix: value };
        const err = validateField("name", value, next);
        setFieldError("name", err);
        return next;
      });
    },
    [validateField, setFieldError],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const result = createEnvironmentSchema.safeParse(formData);
      if (!result.success) {
        validateForm(formData);
        return;
      }
      try {
        await createEnvironment({
          params: { orgName: orgId },
          body: {
            name: result.data.name,
            displayName: result.data.displayName.trim(),
            description: result.data.description?.trim() || undefined,
            dataplaneRef: result.data.dataplaneRef,
            dnsPrefix: result.data.name,
            isProduction: result.data.isProduction,
          },
        });
        onClose();
      } catch {
        // handled by createError
      }
    },
    [formData, validateForm, createEnvironment, orgId, onClose],
  );

  const errorMessage = useMemo(
    () => (createError ? (createError as Error)?.message ?? "Failed to create environment" : null),
    [createError],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<Plus size={24} />} title="Create Environment" onClose={onClose} />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {errorMessage && (
              <Alert severity="error">
                <Typography variant="body2">{errorMessage}</Typography>
              </Alert>
            )}

            <Form.Section>
              <Form.Header>Environment Details</Form.Header>
              <Form.Stack spacing={2}>
                {planes.length > 1 && (
                  <FormControl fullWidth error={Boolean(errors.dataplaneRef)}>
                    <FormLabel required>Data Plane</FormLabel>
                    <Select
                      size="small"
                      value={formData.dataplaneRef}
                      onChange={(e) => handleChange("dataplaneRef", e.target.value as string)}
                      disabled={isPending}
                      error={Boolean(errors.dataplaneRef)}
                    >
                      {planes.map((p: DataPlane) => (
                        <MenuItem key={p.name} value={p.name}>
                          {p.displayName || p.name}
                        </MenuItem>
                      ))}
                    </Select>
                    {errors.dataplaneRef && (
                      <Typography variant="caption" color="error">{errors.dataplaneRef}</Typography>
                    )}
                  </FormControl>
                )}

                <FormControl fullWidth error={Boolean(errors.displayName)}>
                  <FormLabel required>Display Name</FormLabel>
                  <TextField
                    size="small"
                    fullWidth
                    value={formData.displayName}
                    onChange={(e) => handleDisplayNameChange(e.target.value)}
                    placeholder="e.g., Production"
                    error={Boolean(errors.displayName)}
                    helperText={errors.displayName}
                    disabled={isPending}
                  />
                </FormControl>

                <FormControl fullWidth error={Boolean(errors.name)}>
                  <FormLabel required>Name</FormLabel>
                  <TextField
                    size="small"
                    fullWidth
                    value={formData.name}
                    onChange={(e) => handleNameChange(e.target.value)}
                    placeholder="e.g., production"
                    error={Boolean(errors.name)}
                    helperText={errors.name ?? "Lowercase, alphanumeric with hyphens"}
                    disabled={isPending}
                  />
                </FormControl>

                <FormControl fullWidth>
                  <FormLabel>Description</FormLabel>
                  <TextField
                    size="small"
                    fullWidth
                    multiline
                    rows={2}
                    value={formData.description ?? ""}
                    onChange={(e) => handleChange("description", e.target.value)}
                    placeholder="Optional description"
                    disabled={isPending}
                  />
                </FormControl>

                <FormControlLabel
                  control={
                    <Checkbox
                      checked={formData.isProduction ?? false}
                      onChange={(e) => handleChange("isProduction", e.target.checked)}
                      disabled={isPending}
                    />
                  }
                  label="Production environment"
                />
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button variant="outlined" color="inherit" onClick={onClose} disabled={isPending}>
                Cancel
              </Button>
              <Button type="submit" variant="contained" color="primary" disabled={isPending}>
                {isPending ? "Creating..." : "Create"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}
