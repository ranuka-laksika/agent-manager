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
  Form,
  FormControl,
  FormLabel,
  IconButton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Check, Copy, Mail } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useInviteUser } from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const UserInvitePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [email, setEmail] = useState("");
  const [emailError, setEmailError] = useState<string | undefined>();
  const [inviteLink, setInviteLink] = useState<string | undefined>();
  const [copied, setCopied] = useState(false);

  const { mutateAsync: inviteUser, isPending: isInviting, error: inviteError } = useInviteUser();

  const usersPath = orgId
    ? generatePath(
        (absoluteRouteMap.children.org.children as unknown as {
          identities: { children: { users: { path: string } } };
        }).identities.children.users.path,
        { orgId },
      )
    : "#";

  const validateEmail = (value: string) =>
    /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);

  const handleSubmit = async () => {
    if (!email.trim()) {
      setEmailError("Email is required");
      return;
    }
    if (!validateEmail(email.trim())) {
      setEmailError("Enter a valid email address");
      return;
    }
    setEmailError(undefined);

    const result = await inviteUser({
      params: { orgName: orgId },
      body: { email: email.trim() },
    });
    setInviteLink(result.inviteLink);
  };

  const handleCopy = async () => {
    if (!inviteLink) return;
    await navigator.clipboard.writeText(inviteLink);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <PageLayout
      title="Invite User"
      backHref={usersPath}
      backLabel="Back to Users"
      disableIcon
    >
      <Stack spacing={3} sx={{ maxWidth: 700 }}>
        {inviteError != null && (
          <Alert severity="error">
            {(inviteError as Error)?.message ?? "Failed to send invitation"}
          </Alert>
        )}

        {inviteLink == null ? (
          <>
            <Form.Section>
              <Form.Header>Invite Details</Form.Header>
              <Form.Stack spacing={2}>
                <FormControl fullWidth error={Boolean(emailError)}>
                  <FormLabel required>Email Address</FormLabel>
                  <TextField
                    fullWidth
                    type="email"
                    value={email}
                    onChange={(e) => {
                      setEmail(e.target.value);
                      if (emailError) setEmailError(undefined);
                    }}
                    placeholder="user@example.com"
                    autoComplete="off"
                    error={Boolean(emailError)}
                    helperText={emailError}
                  />
                </FormControl>
              </Form.Stack>
            </Form.Section>

            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button variant="outlined" onClick={() => navigate(usersPath)} disabled={isInviting}>
                Cancel
              </Button>
              <Button
                variant="contained"
                startIcon={<Mail />}
                onClick={handleSubmit}
                disabled={isInviting || !email.trim()}
              >
                {isInviting ? "Sending Invite..." : "Send Invite"}
              </Button>
            </Stack>
          </>
        ) : (
          <Form.Section>
            <Form.Header>Invitation Created</Form.Header>
            <Form.Stack spacing={2}>
              <Alert severity="success">
                An invitation has been created for <strong>{email}</strong>. Share the link below with the user to complete registration.
              </Alert>

              <Box>
                <Typography variant="body2" color="text.secondary" mb={1}>
                  Invite Link
                </Typography>
                <Stack direction="row" spacing={1} alignItems="center">
                  <TextField
                    fullWidth
                    value={inviteLink}
                    InputProps={{ readOnly: true }}
                  />
                  <Tooltip title={copied ? "Copied!" : "Copy link"}>
                    <IconButton onClick={handleCopy} size="small">
                      {copied ? <Check size={16} /> : <Copy size={16} />}
                    </IconButton>
                  </Tooltip>
                </Stack>
              </Box>

              <Stack direction="row" spacing={1} justifyContent="flex-end">
                <Button variant="outlined" onClick={() => { setInviteLink(undefined); setEmail(""); }}>
                  Invite Another
                </Button>
                <Button variant="contained" onClick={() => navigate(usersPath)}>
                  Done
                </Button>
              </Stack>
            </Form.Stack>
          </Form.Section>
        )}
      </Stack>
    </PageLayout>
  );
};
