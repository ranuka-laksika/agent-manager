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
  Box,
  Button,
  CircularProgress,
  Stack,
} from "@wso2/oxygen-ui";
import { TextInput, PageLayout } from "@agent-management-platform/views";
import { useNavigate, useParams, generatePath } from "react-router-dom";
import { useCreateUser } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const UserAddPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const identitiesRoute = (absoluteRouteMap.children.org.children as unknown as {
    identities: { children: { users: { path: string } } };
  }).identities;

  const usersPath = orgId
    ? generatePath(identitiesRoute.children.users.path, { orgId })
    : "#";

  const { mutateAsync: createUserMutation, isPending: loading } = useCreateUser();

  const [formData, setFormData] = useState({
    username: "",
    password: "",
    firstName: "",
    lastName: "",
    email: "",
  });
  const [error, setError] = useState<string | null>(null);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    try {
      if (!formData.username) {
        throw new Error("Username is required");
      }
      if (!formData.password) {
        throw new Error("Password is required");
      }

      const attributes: Record<string, string> = {
        username: formData.username,
        password: formData.password,
      };

      if (formData.firstName) {
        attributes.given_name = formData.firstName;
      }
      if (formData.lastName) {
        attributes.family_name = formData.lastName;
      }
      if (formData.email) {
        attributes.email = formData.email;
      }

      await createUserMutation({
        params: { orgName: orgId },
        body: {
          type: "engineer",
          attributes,
        },
      });

      navigate(usersPath);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create user");
    }
  };

  return (
    <PageLayout
      title="Add User"
      backHref={usersPath}
      backLabel="Back to Users"
      disableIcon
    >
      <Box sx={{ maxWidth: 600 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        <form onSubmit={handleSubmit}>
          <Stack spacing={2}>
            <TextInput
              label="Username"
              name="username"
              value={formData.username}
              onChange={handleChange}
              disabled={loading}
              required
            />

            <TextInput
              label="Password"
              name="password"
              type="password"
              value={formData.password}
              onChange={handleChange}
              disabled={loading}
              required
            />

            <TextInput
              label="First Name"
              name="firstName"
              value={formData.firstName}
              onChange={handleChange}
              disabled={loading}
            />

            <TextInput
              label="Last Name"
              name="lastName"
              value={formData.lastName}
              onChange={handleChange}
              disabled={loading}
            />

            <TextInput
              label="Email Address"
              name="email"
              type="email"
              value={formData.email}
              onChange={handleChange}
              disabled={loading}
            />

            <Box sx={{ mt: 3 }}>
              <Button
                variant="contained"
                type="submit"
                disabled={loading || !formData.username.trim() || !formData.password.trim()}
              >
                {loading ? <CircularProgress size={20} /> : "Create User"}
              </Button>
            </Box>
          </Stack>
        </form>
      </Box>
    </PageLayout>
  );
};
