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
  Box,
  Button,
  CircularProgress,
  IconButton,
  ListingTable,
  Stack,
  TablePagination,
  Tooltip,
} from "@wso2/oxygen-ui";
import { Plus, Trash, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteUser,
  useListUsers,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import type { ThunderUser } from "@agent-management-platform/types";

export const UsersPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);

  const { data, isLoading, error } = useListUsers(
    { orgName: orgId },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteUser } = useDeleteUser();
  const { addConfirmation } = useConfirmationDialog();

  const users = useMemo(() => data?.users ?? [], [data]);
  const total = data?.total ?? 0;

  const invitePath = orgId
    ? generatePath(
        (absoluteRouteMap.children.org.children as unknown as {
          identities: { children: { users: { path: string } } };
        }).identities.children.users.path + "/invite",
        { orgId },
      )
    : "#";

  const handleDelete = (user: ThunderUser) => {
    addConfirmation({
      title: "Delete User",
      description: `Are you sure you want to delete "${getAttr(user, "username")}"? This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => deleteUser({ orgName: orgId, userId: user.id }),
    });
  };

  const getAttr = (user: ThunderUser, key: string) =>
    String(user.attributes?.[key] ?? "");

  if (isLoading) {
    return (
      <PageLayout title="Users" disableIcon>
        <Box display="flex" justifyContent="center" mt={4}>
          <CircularProgress />
        </Box>
      </PageLayout>
    );
  }

  return (
    <PageLayout title="Users" disableIcon>
      {error != null && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load users
        </Alert>
      )}

      <Stack direction="row" justifyContent="flex-end" mb={2}>
        <Button
          variant="contained"
          startIcon={<Plus />}
          onClick={() => navigate(invitePath)}
        >
          Invite User
        </Button>
      </Stack>

      <ListingTable.Container>
        {users.length === 0 ? (
          <ListingTable.EmptyState
            illustration={<Users size={64} />}
            title="No users yet"
            description='Click "Invite User" to invite one.'
          />
        ) : (
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Username</ListingTable.Cell>
                <ListingTable.Cell>User ID</ListingTable.Cell>
                <ListingTable.Cell />
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {users.map((user: ThunderUser) => (
                <ListingTable.Row key={user.id}>
                  <ListingTable.Cell>{getAttr(user, "username")}</ListingTable.Cell>
                  <ListingTable.Cell>{user.id}</ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <Tooltip title="Delete user">
                      <IconButton size="small" onClick={() => handleDelete(user)}>
                        <Trash size={16} />
                      </IconButton>
                    </Tooltip>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        )}
        {users.length > 0 && (
          <TablePagination
            component="div"
            count={total}
            page={page}
            rowsPerPage={rowsPerPage}
            onPageChange={(_e, newPage) => setPage(newPage)}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
            rowsPerPageOptions={[5, 10, 25, 50]}
          />
        )}
      </ListingTable.Container>
    </PageLayout>
  );
};
