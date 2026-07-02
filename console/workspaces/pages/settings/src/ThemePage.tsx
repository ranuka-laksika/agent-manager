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

import React, { type ReactNode } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Checkbox,
  Chip,
  Divider,
  Form,
  MenuItem,
  Select,
  Stack,
  Switch,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Bell, Check, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import {
  APP_THEME_OPTIONS,
  type AppThemeKey,
  useAppTheme,
} from "@agent-management-platform/views";

const PREVIEW_SX = { pointerEvents: "none", userSelect: "none" } as const;

const ThemeSection: React.FC<{ label: string; children: ReactNode; divider?: boolean }> = ({
  label,
  children,
  divider = true,
}) => (
  <>
    <Stack spacing={1}>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
      {children}
    </Stack>
    {divider && <Divider />}
  </>
);

const ThemePreview: React.FC = () => (
  <Box sx={PREVIEW_SX}>
    <Form.Section>
      <Typography variant="overline" color="text.secondary">
        Preview
      </Typography>

      <Stack spacing={3} sx={{ mt: 1 }}>
        <ThemeSection label="Buttons">
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            <Button variant="contained" startIcon={<Plus size={16} />}>Create</Button>
            <Button variant="outlined" startIcon={<Check size={16} />}>Confirm</Button>
            <Button variant="text">Cancel</Button>
            <Button variant="contained" color="error" startIcon={<Trash size={16} />}>Delete</Button>
          </Stack>
        </ThemeSection>

        <ThemeSection label="Chips">
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            <Chip label="Active" color="success" size="small" />
            <Chip label="Pending" color="warning" size="small" />
            <Chip label="Inactive" color="default" size="small" />
            <Chip label="Error" color="error" size="small" />
            <Chip label="Admin" color="primary" size="small" variant="outlined" />
          </Stack>
        </ThemeSection>

        <ThemeSection label="Inputs">
          <Stack direction="row" spacing={2} alignItems="flex-start" flexWrap="wrap" useFlexGap>
            <TextField label="Display name" defaultValue="John Doe" size="small" sx={{ width: 200 }} />
            <TextField label="Email" defaultValue="john@example.com" size="small" sx={{ width: 220 }} />
            <TextField label="Error state" defaultValue="invalid" size="small" error helperText="This field is required" sx={{ width: 180 }} />
          </Stack>
        </ThemeSection>

        <ThemeSection label="Tabs">
          <Tabs value={0} sx={{ borderBottom: 1, borderColor: "divider" }}>
            <Tab label="Overview" />
            <Tab label="Members" />
            <Tab label="Settings" />
          </Tabs>
        </ThemeSection>

        <ThemeSection label="Toggles">
          <Stack direction="row" spacing={3} alignItems="center">
            <Stack direction="row" alignItems="center" spacing={0.5}>
              <Switch defaultChecked /><Typography variant="body2">Enabled</Typography>
            </Stack>
            <Stack direction="row" alignItems="center" spacing={0.5}>
              <Switch /><Typography variant="body2">Disabled</Typography>
            </Stack>
            <Stack direction="row" alignItems="center" spacing={0.5}>
              <Checkbox defaultChecked /><Typography variant="body2">Checked</Typography>
            </Stack>
            <Stack direction="row" alignItems="center" spacing={0.5}>
              <Checkbox /><Typography variant="body2">Unchecked</Typography>
            </Stack>
          </Stack>
        </ThemeSection>

        <ThemeSection label="Alerts">
          <Stack spacing={1}>
            <Alert severity="info" icon={<Bell size={16} />}>Your session will expire in 15 minutes.</Alert>
            <Alert severity="success">Role updated successfully.</Alert>
            <Alert severity="warning">You have unsaved changes.</Alert>
            <Alert severity="error">Failed to load users. Please retry.</Alert>
          </Stack>
        </ThemeSection>

        <ThemeSection label="Avatars" divider={false}>
          <Stack direction="row" spacing={1} alignItems="center">
            <Avatar sx={{ width: 32, height: 32, fontSize: 14 }}>JD</Avatar>
            <Avatar sx={{ width: 40, height: 40, fontSize: 16 }}>AK</Avatar>
            <Avatar sx={{ width: 48, height: 48, bgcolor: "primary.main" }}>R</Avatar>
            <Avatar sx={{ width: 56, height: 56, bgcolor: "success.main" }}>M</Avatar>
          </Stack>
        </ThemeSection>
      </Stack>
    </Form.Section>
  </Box>
);

export const ThemePage: React.FC = () => {
  const { themeKey, setThemeKey } = useAppTheme();

  return (
    <Stack spacing={3}>
      <Box>
        <Typography variant="h6">Theme</Typography>
        <Typography variant="body2" color="text.secondary">
          Choose a color theme for the console. Your selection is saved
          automatically.
        </Typography>
      </Box>

      <Form.Section>
        <Form.ElementWrapper label="Color theme" name="theme">
          <Select
            value={themeKey}
            onChange={(e) => setThemeKey(e.target.value as AppThemeKey)}
            sx={{ width: 280 }}
          >
            {APP_THEME_OPTIONS.map(({ key, label, color }) => (
              <MenuItem key={key} value={key}>
                <Stack direction="row" alignItems="center" spacing={1.5}>
                  <Box
                    sx={{
                      width: 16,
                      height: 16,
                      borderRadius: "50%",
                      bgcolor: color,
                      flexShrink: 0,
                      border: "1px solid",
                      borderColor: "divider",
                    }}
                  />
                  <Typography variant="body2">{label}</Typography>
                </Stack>
              </MenuItem>
            ))}
          </Select>
        </Form.ElementWrapper>
      </Form.Section>

      <ThemePreview />
    </Stack>
  );
};

export default ThemePage;
