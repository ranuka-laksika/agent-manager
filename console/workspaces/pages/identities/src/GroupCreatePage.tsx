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
import { useCreateGroup } from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const GroupCreatePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [nameError, setNameError] = useState<string | undefined>();

  const { mutateAsync: createGroup, isPending: isCreating, error: createError } = useCreateGroup();

  const groupsPath = orgId
    ? generatePath(
        (absoluteRouteMap.children.org.children as unknown as {
          identities: { children: { groups: { path: string } } };
        }).identities.children.groups.path,
        { orgId },
      )
    : "#";

  const handleSubmit = async () => {
    if (!name.trim()) {
      setNameError("Name is required");
      return;
    }
    setNameError(undefined);

    await createGroup({
      params: { orgName: orgId },
      body: { name: name.trim(), description: description.trim() || undefined },
    });

    navigate(groupsPath);
  };

  return (
    <PageLayout
      title="Create Group"
      backHref={groupsPath}
      backLabel="Back to Groups"
      disableIcon
    >
      <Stack spacing={3} sx={{ maxWidth: 700 }}>
        {createError != null && (
          <Alert severity="error">
            {(createError as Error)?.message ?? "Failed to create group"}
          </Alert>
        )}

        <Form.Section>
          <Form.Header>Group Details</Form.Header>
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
                placeholder="developers"
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
                placeholder="Describe the group's purpose and membership"
              />
            </FormControl>
          </Form.Stack>
        </Form.Section>

        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button variant="outlined" onClick={() => navigate(groupsPath)} disabled={isCreating}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSubmit}
            disabled={isCreating || !name.trim()}
          >
            {isCreating ? "Creating..." : "Create Group"}
          </Button>
        </Stack>
      </Stack>
    </PageLayout>
  );
};
