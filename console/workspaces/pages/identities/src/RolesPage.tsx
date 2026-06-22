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
import { Edit, Plus, Shield, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteRole,
  useListRoles,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { FadeIn, PageLayout } from "@agent-management-platform/views";
import {
  absoluteRouteMap,
  type ThunderRole,
} from "@agent-management-platform/types";
import { ListingSkeletonRows } from "./components/ListingSkeletonRows";

export const RolesPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [hoveredId, setHoveredId] = useState<string | null>(null);

  const { data, isLoading, error } = useListRoles(
    { orgName: orgId },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteRole } = useDeleteRole();
  const { addConfirmation } = useConfirmationDialog();

  const roles = useMemo(() => data?.roles ?? [], [data]);
  const total = data?.total ?? 0;

  useEffect(() => {
    if (roles.length === 0 && total > 0) {
      const lastPage = Math.max(0, Math.ceil(total / rowsPerPage) - 1);
      if (page !== lastPage) {
        setPage(lastPage);
      }
    }
  }, [roles.length, total, page, rowsPerPage]);

  const rolesBasePath = (
    absoluteRouteMap.children.org.children as unknown as {
      identities: { children: { roles: { path: string } } };
    }
  ).identities.children.roles.path;

  const createPath = orgId
    ? generatePath(rolesBasePath + "/create", { orgId })
    : "#";

  const editRolePath = (roleId: string) =>
    orgId
      ? generatePath(rolesBasePath + "/:roleId/edit", { orgId, roleId })
      : "#";

  const handleDelete = (role: ThunderRole) => {
    addConfirmation({
      title: "Delete Role",
      description: `Are you sure you want to delete "${role.name}"? This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => deleteRole({ orgName: orgId, roleId: role.id }),
    });
  };

  return (
    <PageLayout title="Roles" disableIcon>
      {error != null && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load roles
        </Alert>
      )}

      <Stack direction="row" justifyContent="flex-end" mb={2}>
        <Button
          variant="contained"
          startIcon={<Plus />}
          onClick={() => navigate(createPath)}
        >
          Create Role
        </Button>
      </Stack>

      <ListingTable.Container disablePaper>
        {!isLoading && total === 0 ? (
          <ListingTable.EmptyState
            illustration={<Shield size={64} />}
            title="No roles yet"
            description='Click "Create Role" to add one.'
          />
        ) : (
          <ListingTable variant="card">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Description</ListingTable.Cell>
                <ListingTable.Cell align="right" width="120px" />
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {isLoading && <ListingSkeletonRows rows={rowsPerPage} />}
              {!isLoading &&
                roles.map((role: ThunderRole) => (
                  <ListingTable.Row
                    key={role.id}
                    variant="card"
                    hover
                    onMouseEnter={() => setHoveredId(role.id)}
                    onMouseLeave={() => setHoveredId(null)}
                    onFocus={() => setHoveredId(role.id)}
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
                          {role.name.charAt(0).toUpperCase() || "R"}
                        </Avatar>
                        <Typography variant="body2" fontWeight={500} noWrap>
                          {role.name}
                        </Typography>
                      </Stack>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="body2" color="text.secondary">
                        {role.description ?? "—"}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell align="right">
                      {hoveredId === role.id && (
                        <FadeIn>
                          <Stack
                            direction="row"
                            spacing={0.5}
                            justifyContent="flex-end"
                          >
                            <Tooltip title="Edit role">
                              <IconButton
                                size="small"
                                onClick={() => navigate(editRolePath(role.id))}
                              >
                                <Edit size={16} />
                              </IconButton>
                            </Tooltip>
                            {!role.isReadOnly && (
                              <Tooltip title="Delete role">
                                <IconButton
                                  size="small"
                                  color="error"
                                  onClick={() => handleDelete(role)}
                                >
                                  <Trash size={16} />
                                </IconButton>
                              </Tooltip>
                            )}
                          </Stack>
                        </FadeIn>
                      )}
                    </ListingTable.Cell>
                  </ListingTable.Row>
                ))}
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
