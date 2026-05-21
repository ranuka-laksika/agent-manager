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

import React, { useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  Divider,
  IconButton,
  ListingTable,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useListUsers,
  useGetGroupMembers,
  useGetGroupRoles,
  useAddGroupMembers,
  useRemoveGroupMembers,
} from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import type { ThunderUser, ThunderRole } from "@agent-management-platform/types";

export const GroupEditPage: React.FC = () => {
  const { orgId, groupId } = useParams<{ orgId: string; groupId: string }>();
  const navigate = useNavigate();

  const { data: membersData, isLoading: isLoadingMembers } = useGetGroupMembers(
    { orgName: orgId, groupId: groupId ?? "" },
    { offset: 0, limit: 100 },
  );
  const { data: rolesData } = useGetGroupRoles({
    orgName: orgId,
    groupId: groupId ?? "",
  });
  const { data: allUsersData, isLoading: isLoadingUsers } = useListUsers(
    { orgName: orgId },
    { offset: 0, limit: 100 },
  );

  const { mutateAsync: addMembers } = useAddGroupMembers();
  const { mutateAsync: removeMembers } = useRemoveGroupMembers();

  const initialMembers: ThunderUser[] = useMemo(
    () => membersData?.users ?? [],
    [membersData],
  );
  const roles: ThunderRole[] = useMemo(() => rolesData?.roles ?? [], [rolesData]);
  const allUsers: ThunderUser[] = useMemo(() => allUsersData?.users ?? [], [allUsersData]);

  // Track local edits — no useEffect needed
  const [pendingAdds, setPendingAdds] = useState<ThunderUser[]>([]);
  const [removedIds, setRemovedIds] = useState<Set<string>>(new Set());

  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();

  const groupsPath = orgId
    ? generatePath(
        (absoluteRouteMap.children.org.children as unknown as {
          identities: { children: { groups: { path: string } } };
        }).identities.children.groups.path,
        { orgId },
      )
    : "#";

  // Displayed members = server state + local adds - local removes
  const displayedMembers = useMemo(() => {
    const base = initialMembers.filter((u) => !removedIds.has(u.id));
    return [...base, ...pendingAdds];
  }, [initialMembers, pendingAdds, removedIds]);

  const displayedMemberIds = useMemo(
    () => new Set(displayedMembers.map((u) => u.id)),
    [displayedMembers],
  );

  const availableUsers = useMemo(
    () => allUsers.filter((u) => !displayedMemberIds.has(u.id)),
    [allUsers, displayedMemberIds],
  );

  const getUsername = (user: ThunderUser) =>
    String(user.attributes?.["username"] ?? user.id ?? "");

  const handleAddUser = (_e: React.SyntheticEvent, value: ThunderUser | null) => {
    if (!value) return;
    // If it was previously removed, just un-remove it
    if (removedIds.has(value.id)) {
      setRemovedIds((prev) => {
        const next = new Set(prev);
        next.delete(value.id);
        return next;
      });
    } else {
      setPendingAdds((prev) => [...prev, value]);
    }
  };

  const handleRemoveUser = (userId: string) => {
    if (pendingAdds.find((u) => u.id === userId)) {
      setPendingAdds((prev) => prev.filter((u) => u.id !== userId));
    } else {
      setRemovedIds((prev) => new Set([...prev, userId]));
    }
  };

  const handleSave = async () => {
    if (!orgId || !groupId) return;
    setSaveError(undefined);
    setIsSaving(true);
    try {
      for (const u of pendingAdds) {
        await addMembers({ params: { orgName: orgId, groupId }, body: { userIds: [u.id] } });
      }
      for (const id of removedIds) {
        await removeMembers({ params: { orgName: orgId, groupId }, body: { userIds: [id] } });
      }
      navigate(groupsPath);
    } catch {
      setSaveError("Failed to update group members. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading = isLoadingMembers || isLoadingUsers;

  if (isLoading) {
    return (
      <PageLayout title="Edit Group" disableIcon>
        <Box display="flex" justifyContent="center" mt={4}>
          <CircularProgress />
        </Box>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title="Edit Group"
      backHref={groupsPath}
      backLabel="Back to Groups"
      disableIcon
    >
      <Stack spacing={4} sx={{ maxWidth: 800 }}>
        {saveError != null && <Alert severity="error">{saveError}</Alert>}

        {/* Users section */}
        <Box>
          <Typography variant="subtitle1" fontWeight={600} mb={1}>
            Users
          </Typography>
          <Typography variant="body2" color="text.secondary" mb={2}>
            Search and add users to this group.
          </Typography>
          <Divider sx={{ mb: 2 }} />

          <Autocomplete
            options={availableUsers}
            getOptionLabel={(option) => getUsername(option as ThunderUser)}
            onChange={handleAddUser}
            value={null}
            renderInput={(params) => (
              <TextField {...params} placeholder="Search users..." label="Add User" />
            )}
            noOptionsText="No users available"
            sx={{ mb: 2 }}
          />

          {displayedMembers.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No members yet. Search and add users above.
            </Typography>
          ) : (
            <ListingTable.Container>
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Username</ListingTable.Cell>
                    <ListingTable.Cell>User ID</ListingTable.Cell>
                    <ListingTable.Cell />
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {displayedMembers.map((user) => (
                    <ListingTable.Row key={user.id}>
                      <ListingTable.Cell>{getUsername(user)}</ListingTable.Cell>
                      <ListingTable.Cell>{user.id}</ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        <Tooltip title="Remove from group">
                          <IconButton
                            size="small"
                            onClick={() => handleRemoveUser(user.id)}
                          >
                            <Trash size={16} />
                          </IconButton>
                        </Tooltip>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          )}
        </Box>

        {/* Roles section */}
        <Box>
          <Typography variant="subtitle1" fontWeight={600} mb={1}>
            Assigned Roles
          </Typography>
          <Typography variant="body2" color="text.secondary" mb={2}>
            Roles currently assigned to this group. Manage role assignments from the Roles page.
          </Typography>
          <Divider sx={{ mb: 2 }} />

          {rolesData == null ? (
            <CircularProgress size={20} />
          ) : roles.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No roles assigned to this group.
            </Typography>
          ) : (
            <ListingTable.Container>
              <ListingTable>
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Name</ListingTable.Cell>
                    <ListingTable.Cell>Description</ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {roles.map((role) => (
                    <ListingTable.Row key={role.id}>
                      <ListingTable.Cell>{role.name}</ListingTable.Cell>
                      <ListingTable.Cell>{role.description ?? "-"}</ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
            </ListingTable.Container>
          )}
        </Box>

        <Stack direction="row" spacing={1} justifyContent="flex-end">
          <Button
            variant="outlined"
            onClick={() => navigate(groupsPath)}
            disabled={isSaving}
          >
            Cancel
          </Button>
          <Button variant="contained" onClick={handleSave} disabled={isSaving}>
            {isSaving ? "Saving..." : "Save Changes"}
          </Button>
        </Stack>
      </Stack>
    </PageLayout>
  );
};
