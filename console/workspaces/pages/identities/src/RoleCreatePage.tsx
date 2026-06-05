/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useState } from "react";
import {
  Alert,
  Button,
  Form,
  FormControl,
  FormLabel,
  Stack,
  TextField,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useCreateRole } from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const RoleCreatePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [nameError, setNameError] = useState<string | undefined>();

  const { mutateAsync: createRole, isPending: isCreating, error: createError } = useCreateRole();

  const rolesPath = orgId
    ? generatePath(
        (absoluteRouteMap.children.org.children as unknown as {
          identities: { children: { roles: { path: string } } };
        }).identities.children.roles.path,
        { orgId },
      )
    : "#";

  const handleSubmit = async () => {
    if (!name.trim()) {
      setNameError("Name is required");
      return;
    }
    setNameError(undefined);

    try {
      await createRole({
        params: { orgName: orgId },
        body: { name: name.trim(), description: description.trim() || undefined },
      });
      navigate(rolesPath);
    } catch {
      // createError state is set by React Query and displayed in the Alert above
    }
  };

  return (
    <PageLayout
      title="Create Role"
      backHref={rolesPath}
      backLabel="Back to Roles"
      disableIcon
    >
      <Stack spacing={3} sx={{ maxWidth: 700 }}>
        {createError != null && (
          <Alert severity="error">
            {(createError as Error)?.message ?? "Failed to create role"}
          </Alert>
        )}

        <Form.Section>
          <Form.Header>Role Details</Form.Header>
          <Form.Stack spacing={2}>
            <FormControl fullWidth error={Boolean(nameError)}>
              <FormLabel required>Name</FormLabel>
              <TextField
                fullWidth
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  if (nameError) setNameError(undefined);
                }}
                placeholder="admin"
                autoComplete="off"
                error={Boolean(nameError)}
                helperText={nameError}
              />
            </FormControl>

            <FormControl fullWidth>
              <FormLabel>Description</FormLabel>
              <TextField
                fullWidth
                multiline
                rows={3}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Describe the role's purpose and permissions"
              />
            </FormControl>
          </Form.Stack>
        </Form.Section>

        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="outlined" onClick={() => navigate(rolesPath)} disabled={isCreating}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSubmit}
            disabled={isCreating || !name.trim()}
          >
            {isCreating ? "Creating..." : "Create Role"}
          </Button>
        </Stack>
      </Stack>
    </PageLayout>
  );
};
