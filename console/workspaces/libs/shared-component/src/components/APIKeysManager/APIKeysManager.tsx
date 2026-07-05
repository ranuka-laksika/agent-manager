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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Copy,
  Key,
  Plus,
  Trash2 as DeleteIcon,
} from "@wso2/oxygen-ui-icons-react";
import type { APIKeyInfo, SecurityConfig } from "@agent-management-platform/types";

/**
 * Returns true when API-key authentication is enabled in a resource's security
 * config. Mirrors the shape persisted by the LLM provider / MCP proxy Security
 * tabs (security.apiKey.enabled === true).
 */
export function isApiKeyAuthEnabled(security?: SecurityConfig | null): boolean {
  return security?.enabled !== false && security?.apiKey?.enabled === true;
}

export interface CreateAPIKeyInput {
  displayName: string;
  /** RFC3339 expiry timestamp. */
  expiresAt: string;
}

export interface APIKeysManagerProps {
  /** The user-managed keys to display. */
  keys: APIKeyInfo[] | undefined;
  isLoading: boolean;
  isError?: boolean;
  /** True while a create request is in flight. */
  isCreating: boolean;
  /** True while a revoke request is in flight. */
  isRevoking?: boolean;
  /** Empty-state copy, e.g. "Create an API key to authenticate requests to this agent." */
  emptyDescription: string;
  /** Creates a key and resolves with the one-time plaintext key (or undefined). */
  onCreate: (input: CreateAPIKeyInput) => Promise<string | undefined>;
  /** Revokes the key with the given name. */
  onRevoke: (keyName: string) => void;
}

function CreateAPIKeyDialog({
  open,
  onClose,
  isCreating,
  onCreate,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  isCreating: boolean;
  onCreate: (input: CreateAPIKeyInput) => Promise<string | undefined>;
  onCreated: (key: string) => void;
}) {
  const defaultExpiry = () => {
    const d = new Date();
    d.setMonth(d.getMonth() + 1);
    return d.toISOString().slice(0, 10);
  };
  const [displayName, setDisplayName] = useState("");
  const [expiresAt, setExpiresAt] = useState(defaultExpiry);

  const trimmedDisplayName = displayName.trim();
  const canSubmit = trimmedDisplayName.length > 0 && expiresAt.length > 0;

  const handleClose = () => {
    setDisplayName("");
    setExpiresAt(defaultExpiry());
    onClose();
  };

  const handleCreate = async () => {
    if (!canSubmit) return;
    // Interpret the picked date as the end of that day in the user's local time
    // zone (not UTC), so the selected calendar day is preserved regardless of
    // the user's offset. toISOString() then yields the correct RFC3339 instant.
    const [year, month, day] = expiresAt.split("-").map(Number);
    const expiresAtRFC3339 = new Date(
      year,
      month - 1,
      day,
      23,
      59,
      59,
      999,
    ).toISOString();
    try {
      const key = await onCreate({
        displayName: trimmedDisplayName,
        expiresAt: expiresAtRFC3339,
      });
      if (key) onCreated(key);
      handleClose();
    } catch {
      // The error is surfaced by the mutation's notification handler; keep the
      // dialog open so the user can retry.
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create API Key</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Display name"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            fullWidth
            required
            size="small"
            placeholder="production key"
          />
          <TextField
            label="Expires"
            type="date"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            fullWidth
            required
            size="small"
            error={expiresAt.length === 0}
            slotProps={{ inputLabel: { shrink: true } }}
            helperText="Key expires at end of the selected day"
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button variant="outlined" onClick={handleClose} disabled={isCreating}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={() => void handleCreate()}
          disabled={isCreating || !canSubmit}
          startIcon={isCreating ? <CircularProgress size={16} /> : undefined}
        >
          {isCreating ? "Creating..." : "Create"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function NewKeyBanner({
  apiKey,
  onDismiss,
}: {
  apiKey: string;
  onDismiss: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(apiKey).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <Alert
      severity="success"
      onClose={onDismiss}
      sx={{ mb: 2, "& .MuiAlert-message": { flexGrow: 1 } }}
    >
      <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
        You will only see this key once. Copy it now.
      </Typography>
      <Box display="flex" alignItems="center" gap={1}>
        <TextField
          size="small"
          fullWidth
          value={apiKey}
          slotProps={{ input: { readOnly: true } }}
        />
        <Tooltip title={copied ? "Copied!" : "Copy"}>
          <IconButton size="small" onClick={handleCopy} aria-label="Copy API key">
            <Copy size={16} />
          </IconButton>
        </Tooltip>
      </Box>
    </Alert>
  );
}

function APIKeyRow({
  apiKey,
  onRevoke,
  isRevoking,
}: {
  apiKey: APIKeyInfo;
  onRevoke: (keyName: string) => void;
  isRevoking?: boolean;
}) {
  return (
    <Box
      display="flex"
      alignItems="center"
      justifyContent="space-between"
      px={2}
      py={1.5}
      sx={{ borderBottom: "1px solid", borderColor: "divider" }}
    >
      <Box display="flex" alignItems="center" gap={2}>
        <Key size={18} />
        <Box>
          <Typography variant="body2" fontWeight={500}>
            {apiKey.displayName || apiKey.name}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {apiKey.maskedApiKey}
            {apiKey.expiresAt &&
              ` · Expires ${new Date(apiKey.expiresAt).toLocaleDateString()}`}
          </Typography>
        </Box>
      </Box>
      <Box display="flex" alignItems="center" gap={1}>
        <Chip
          label={apiKey.status}
          size="small"
          color={apiKey.status === "active" ? "success" : "default"}
        />
        <Tooltip title="Revoke">
          <span>
            <IconButton
              size="small"
              color="error"
              onClick={() => onRevoke(apiKey.name)}
              disabled={isRevoking}
              aria-label="Revoke API key"
            >
              <DeleteIcon size={16} />
            </IconButton>
          </span>
        </Tooltip>
      </Box>
    </Box>
  );
}

/**
 * Presentational manager for an artifact's user-managed API keys: empty state,
 * create dialog (display name + expiry), one-time key banner, key list and
 * revoke. Data and mutations are supplied by the parent so the same UI can back
 * agents, LLM providers and MCP proxies without duplicating it.
 */
export function APIKeysManager({
  keys,
  isLoading,
  isError = false,
  isCreating,
  isRevoking = false,
  emptyDescription,
  onCreate,
  onRevoke,
}: APIKeysManagerProps) {
  const [createOpen, setCreateOpen] = useState(false);
  const [newKeyValue, setNewKeyValue] = useState<string | null>(null);

  if (isLoading) {
    return <Skeleton variant="rectangular" width="100%" height={200} />;
  }

  const hasKeys = !!keys && keys.length > 0;

  return (
    <Box>
      {hasKeys && (
        <Stack direction="row" justifyContent="flex-end" sx={{ mb: 2 }}>
          <Button
            variant="contained"
            startIcon={<Plus size={16} />}
            onClick={() => setCreateOpen(true)}
          >
            Create
          </Button>
        </Stack>
      )}

      {newKeyValue && (
        <NewKeyBanner
          apiKey={newKeyValue}
          onDismiss={() => setNewKeyValue(null)}
        />
      )}

      {isError ? (
        <Alert severity="error">
          Failed to load API keys. Please refresh the page.
        </Alert>
      ) : hasKeys ? (
        <Box
          sx={{ border: "1px solid", borderColor: "divider", borderRadius: 1 }}
        >
          {keys!.map((key) => (
            <APIKeyRow
              key={key.name}
              apiKey={key}
              onRevoke={onRevoke}
              isRevoking={isRevoking}
            />
          ))}
        </Box>
      ) : (
        <Box
          display="flex"
          flexDirection="column"
          alignItems="center"
          justifyContent="center"
          py={8}
          gap={2}
        >
          <Key size={48} />
          <Typography variant="h6">No API keys</Typography>
          <Typography variant="body2" color="text.secondary">
            {emptyDescription}
          </Typography>
          <Button
            variant="contained"
            startIcon={<Plus size={16} />}
            onClick={() => setCreateOpen(true)}
          >
            Create API Key
          </Button>
        </Box>
      )}

      <CreateAPIKeyDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        isCreating={isCreating}
        onCreate={onCreate}
        onCreated={(key) => setNewKeyValue(key)}
      />
    </Box>
  );
}

export default APIKeysManager;
