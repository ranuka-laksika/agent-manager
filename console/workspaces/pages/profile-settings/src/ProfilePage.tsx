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

import React, { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Form,
  Stack,
  Tabs,
  Tab,
  CircularProgress,
} from "@wso2/oxygen-ui";
import { PageLayout, TextInput } from "@agent-management-platform/views";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useUpdateUserProfile, useUpdateUserCredentials } from "@agent-management-platform/api-client";

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index } = props;

  return (
    <div hidden={value !== index} style={{ width: "100%" }}>
      {value === index && <Box sx={{ pt: 3 }}>{children}</Box>}
    </div>
  );
}

export const ProfilePage: React.FC = () => {
  const { userInfo } = useAuthHooks();
  const [activeTab, setActiveTab] = useState(0);

  // Profile update state
  const [profileData, setProfileData] = useState({
    username: userInfo?.username || "",
    firstname: userInfo?.givenName || "",
    lastname: userInfo?.familyName || "",
    email: userInfo?.email || "",
  });
  const [profileErrors, setProfileErrors] = useState<Record<string, string>>({});
  const { mutateAsync: updateProfile, isPending: isUpdatingProfile, error: profileError } = useUpdateUserProfile();

  // Credential update state
  const [credentialData, setCredentialData] = useState({
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const [credentialErrors, setCredentialErrors] = useState<Record<string, string>>({});
  const { mutateAsync: updateCredentials, isPending: isUpdatingCredentials, error: credentialError } = useUpdateUserCredentials();

  const [successMessage, setSuccessMessage] = useState("");

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
    setActiveTab(newValue);
    setSuccessMessage("");
  };

  const validateProfile = (): boolean => {
    const errors: Record<string, string> = {};
    if (!profileData.username.trim()) {
      errors.username = "Username is required";
    }
    if (!profileData.email.trim()) {
      errors.email = "Email is required";
    } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(profileData.email)) {
      errors.email = "Invalid email format";
    }
    setProfileErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const validateCredentials = (): boolean => {
    const errors: Record<string, string> = {};
    if (!credentialData.newPassword.trim()) {
      errors.newPassword = "New password is required";
    } else if (credentialData.newPassword.length < 8) {
      errors.newPassword = "Password must be at least 8 characters";
    }
    if (credentialData.newPassword !== credentialData.confirmPassword) {
      errors.confirmPassword = "Passwords do not match";
    }
    setCredentialErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const handleProfileSubmit = async () => {
    if (!validateProfile()) return;

    try {
      await updateProfile({
        body: {
          attributes: {
            username: profileData.username.trim(),
            firstname: profileData.firstname.trim(),
            lastname: profileData.lastname.trim(),
            email: profileData.email.trim(),
          },
        },
      });
      setSuccessMessage("Profile updated successfully");
      setProfileErrors({});
    } catch {
      // Error is shown via profileError state
    }
  };

  const handleCredentialSubmit = async () => {
    if (!validateCredentials()) return;

    try {
      await updateCredentials(
        {
          attributes: {
            password: credentialData.newPassword,
          },
        }
      );
      setSuccessMessage("Password updated successfully");
      setCredentialData({
        currentPassword: "",
        newPassword: "",
        confirmPassword: "",
      });
      setCredentialErrors({});
    } catch {
      // Error is shown via credentialError state
    }
  };

  return (
    <PageLayout
      title="Profile Settings"
      disableIcon
    >
      <Box sx={{ maxWidth: 700 }}>
        {successMessage ? (
          <Alert severity="success" sx={{ mb: 2 }}>
            {successMessage}
          </Alert>
        ) : null}

        {profileError || credentialError ? (
          <Alert severity="error" sx={{ mb: 2 }}>
            {(profileError as Error)?.message || (credentialError as Error)?.message || "An error occurred"}
          </Alert>
        ) : null}

        <Tabs value={activeTab} onChange={handleTabChange}>
          <Tab label="Profile Information" />
          <Tab label="Change Password" />
        </Tabs>

        {/* Profile Information Tab */}
        <TabPanel value={activeTab} index={0}>
          <Form.Stack spacing={3}>
            <Form.Section>
              <Form.Header>Account Information</Form.Header>
              <Form.Stack spacing={2}>
                <TextInput
                  label="Username"
                  required
                  value={profileData.username}
                  onChange={(e) => {
                    setProfileData({ ...profileData, username: e.target.value });
                    if (profileErrors.username) {
                      setProfileErrors({ ...profileErrors, username: "" });
                    }
                  }}
                  error={Boolean(profileErrors.username)}
                  helperText={profileErrors.username}
                />

                <TextInput
                  label="Email"
                  type="email"
                  required
                  value={profileData.email}
                  onChange={(e) => {
                    setProfileData({ ...profileData, email: e.target.value });
                    if (profileErrors.email) {
                      setProfileErrors({ ...profileErrors, email: "" });
                    }
                  }}
                  error={Boolean(profileErrors.email)}
                  helperText={profileErrors.email}
                />
              </Form.Stack>
            </Form.Section>

            <Form.Section>
              <Form.Header>Personal Information</Form.Header>
              <Form.Stack spacing={2}>
                <Box
                  sx={{
                    display: "grid",
                    gap: 2,
                    gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
                  }}
                >
                  <TextInput
                    label="First Name"
                    value={profileData.firstname}
                    onChange={(e) => setProfileData({ ...profileData, firstname: e.target.value })}
                  />

                  <TextInput
                    label="Last Name"
                    value={profileData.lastname}
                    onChange={(e) => setProfileData({ ...profileData, lastname: e.target.value })}
                  />
                </Box>
              </Form.Stack>
            </Form.Section>

            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button
                variant="contained"
                onClick={handleProfileSubmit}
                disabled={isUpdatingProfile}
              >
                {isUpdatingProfile ? <CircularProgress size={24} /> : "Save Changes"}
              </Button>
            </Stack>
          </Form.Stack>
        </TabPanel>

        {/* Change Password Tab */}
        <TabPanel value={activeTab} index={1}>
          <Form.Stack spacing={3}>
            <Form.Section>
              <Form.Header>Update Password</Form.Header>
              <Form.Stack spacing={2}>
                <TextInput
                  label="New Password"
                  type="password"
                  showPasswordToggle
                  required
                  value={credentialData.newPassword}
                  onChange={(e) => {
                    setCredentialData({ ...credentialData, newPassword: e.target.value });
                    if (credentialErrors.newPassword) {
                      setCredentialErrors({ ...credentialErrors, newPassword: "" });
                    }
                  }}
                  error={Boolean(credentialErrors.newPassword)}
                  helperText={credentialErrors.newPassword || "Minimum 8 characters"}
                />

                <TextInput
                  label="Confirm Password"
                  type="password"
                  showPasswordToggle
                  required
                  value={credentialData.confirmPassword}
                  onChange={(e) => {
                    setCredentialData({ ...credentialData, confirmPassword: e.target.value });
                    if (credentialErrors.confirmPassword) {
                      setCredentialErrors({ ...credentialErrors, confirmPassword: "" });
                    }
                  }}
                  error={Boolean(credentialErrors.confirmPassword)}
                  helperText={credentialErrors.confirmPassword}
                />
              </Form.Stack>
            </Form.Section>

            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button
                variant="contained"
                onClick={handleCredentialSubmit}
                disabled={isUpdatingCredentials || !credentialData.newPassword}
              >
                {isUpdatingCredentials ? <CircularProgress size={24} /> : "Update Password"}
              </Button>
            </Stack>
          </Form.Stack>
        </TabPanel>
      </Box>
    </PageLayout>
  );
};
