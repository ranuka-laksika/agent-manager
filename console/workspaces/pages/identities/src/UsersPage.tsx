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

import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Button,
  IconButton,
  ListingTable,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteUser,
  useListUsers,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { FadeIn, PageLayout } from "@agent-management-platform/views";
import {
  absoluteRouteMap,
  type ThunderUser,
} from "@agent-management-platform/types";
import { ListingSkeletonRows } from "./components/ListingSkeletonRows";

export const UsersPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [hoveredId, setHoveredId] = useState<string | null>(null);

  const { data, isLoading, error } = useListUsers(
    { orgName: orgId },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteUser } = useDeleteUser();
  const { addConfirmation } = useConfirmationDialog();

  const users = useMemo(() => data?.users ?? [], [data]);
  const total = data?.total ?? 0;

  useEffect(() => {
    if (users.length === 0 && total > 0) {
      const lastPage = Math.max(0, Math.ceil(total / rowsPerPage) - 1);
      if (page !== lastPage) {
        setPage(lastPage);
      }
    }
  }, [users.length, total, page, rowsPerPage]);

  const identitiesRoute = (
    absoluteRouteMap.children.org.children as unknown as {
      identities: { children: { users: { path: string } } };
    }
  ).identities;

  const invitePath = orgId
    ? generatePath(identitiesRoute.children.users.path + "/invite", { orgId })
    : "#";

  const addUserPath = orgId
    ? generatePath(identitiesRoute.children.users.path + "/add", { orgId })
    : "#";

  const editUserPath = (userId: string) =>
    orgId
      ? generatePath(identitiesRoute.children.users.path + "/:userId", {
          orgId,
          userId,
        })
      : "#";

  const getAttr = (user: ThunderUser, key: string) =>
    String(user.attributes?.[key] ?? "");

  const handleDelete = (user: ThunderUser) => {
    addConfirmation({
      title: "Delete User",
      description:
        `Are you sure you want to delete "${getAttr(user, "username")}"?` +
        " This action cannot be undone.",
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => deleteUser({ orgName: orgId, userId: user.id }),
    });
  };

  return (
    <PageLayout title="Users" disableIcon>
      {error != null && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load users
        </Alert>
      )}

      <Stack direction="row" spacing={2} justifyContent="flex-end" mb={2}>
        <Button
          variant="outlined"
          startIcon={<Plus />}
          onClick={() => navigate(addUserPath)}
        >
          Add User
        </Button>
        <Button
          variant="contained"
          startIcon={<Plus />}
          onClick={() => navigate(invitePath)}
        >
          Invite User
        </Button>
      </Stack>

      <ListingTable.Container disablePaper>
        {!isLoading && total === 0 ? (
          <ListingTable.EmptyState
            illustration={<Users size={64} />}
            title="No users yet"
            description='Click "Add User" to create one or "Invite User" to invite someone.'
          />
        ) : (
          <ListingTable variant="card">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Username</ListingTable.Cell>
                <ListingTable.Cell>User ID</ListingTable.Cell>
                <ListingTable.Cell align="right" width="120px" />
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {isLoading && <ListingSkeletonRows rows={rowsPerPage} />}
              {!isLoading &&
                users.map((user: ThunderUser) => {
                  const username = getAttr(user, "username");
                  return (
                    <ListingTable.Row
                      key={user.id}
                      variant="card"
                      hover
                      clickable
                      onClick={() => navigate(editUserPath(user.id))}
                      onMouseEnter={() => setHoveredId(user.id)}
                      onMouseLeave={() => setHoveredId(null)}
                      onFocus={() => setHoveredId(user.id)}
                      onBlur={(e) => {
                        if (
                          !e.currentTarget.contains(
                            e.relatedTarget as Node | null,
                          )
                        ) {
                          setHoveredId(null);
                        }
                      }}
                    >
                      <ListingTable.Cell>
                        <Stack direction="row" alignItems="center" spacing={2}>
                          <Avatar
                            sx={{
                              bgcolor: "primary.main",
                              color: "primary.contrastText",
                              fontSize: 16,
                              height: 36,
                              width: 36,
                              flexShrink: 0,
                            }}
                          >
                            {username.charAt(0).toUpperCase() || "U"}
                          </Avatar>
                          <Typography variant="body2" fontWeight={500} noWrap>
                            {username}
                          </Typography>
                        </Stack>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          noWrap
                        >
                          {user.id}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        {hoveredId === user.id && (
                          <FadeIn>
                            <Stack
                              direction="row"
                              spacing={0.5}
                              justifyContent="flex-end"
                            >
                              <Tooltip title="Delete user">
                                <IconButton
                                  size="small"
                                  color="error"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleDelete(user);
                                  }}
                                >
                                  <Trash size={16} />
                                </IconButton>
                              </Tooltip>
                            </Stack>
                          </FadeIn>
                        )}
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  );
                })}
            </ListingTable.Body>
          </ListingTable>
        )}
        {!isLoading && total > 0 && (
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
