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
import { Edit, Folder, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteGroup,
  useListGroups,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { FadeIn, PageLayout } from "@agent-management-platform/views";
import {
  absoluteRouteMap,
  type ThunderGroup,
} from "@agent-management-platform/types";
import { ListingSkeletonRows } from "./components/ListingSkeletonRows";

export const GroupsPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [hoveredId, setHoveredId] = useState<string | null>(null);

  const { data, isLoading, error } = useListGroups(
    { orgName: orgId },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteGroup } = useDeleteGroup();
  const { addConfirmation } = useConfirmationDialog();

  const groups = useMemo(() => data?.groups ?? [], [data]);
  const total = data?.total ?? 0;

  useEffect(() => {
    if (groups.length === 0 && total > 0) {
      const lastPage = Math.max(0, Math.ceil(total / rowsPerPage) - 1);
      if (page !== lastPage) {
        setPage(lastPage);
      }
    }
  }, [groups.length, total, page, rowsPerPage]);

  const identitiesRoute = (
    absoluteRouteMap.children.org.children as unknown as {
      identities: { children: { groups: { path: string } } };
    }
  ).identities;

  const createPath = orgId
    ? generatePath(identitiesRoute.children.groups.path + "/create", { orgId })
    : "#";

  const editGroupPath = (groupId: string) =>
    orgId
      ? generatePath(identitiesRoute.children.groups.path + "/:groupId/edit", {
          orgId,
          groupId,
        })
      : "#";

  const handleDelete = (group: ThunderGroup) => {
    addConfirmation({
      title: "Delete Group",
      description: `Are you sure you want to delete "${group.name}"? This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => deleteGroup({ orgName: orgId, groupId: group.id }),
    });
  };

  return (
    <PageLayout title="Groups" disableIcon>
      {error != null && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load groups
        </Alert>
      )}

      <Stack direction="row" justifyContent="flex-end" mb={2}>
        <Button
          variant="contained"
          startIcon={<Plus />}
          onClick={() => navigate(createPath)}
        >
          Create Group
        </Button>
      </Stack>

      <ListingTable.Container disablePaper>
        {!isLoading && total === 0 ? (
          <ListingTable.EmptyState
            illustration={<Folder size={64} />}
            title="No groups yet"
            description='Click "Create Group" to add one.'
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
                groups.map((group: ThunderGroup) => (
                  <ListingTable.Row
                    key={group.id}
                    variant="card"
                    hover
                    onMouseEnter={() => setHoveredId(group.id)}
                    onMouseLeave={() => setHoveredId(null)}
                    onFocus={() => setHoveredId(group.id)}
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
                          {group.name.charAt(0).toUpperCase() || "G"}
                        </Avatar>
                        <Typography variant="body2" fontWeight={500} noWrap>
                          {group.name}
                        </Typography>
                      </Stack>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="body2" color="text.secondary">
                        {group.description ?? "—"}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell align="right">
                      {hoveredId === group.id && (
                        <FadeIn>
                          <Stack
                            direction="row"
                            spacing={0.5}
                            justifyContent="flex-end"
                          >
                            <Tooltip title="Edit group">
                              <IconButton
                                size="small"
                                onClick={() =>
                                  navigate(editGroupPath(group.id))
                                }
                              >
                                <Edit size={16} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete group">
                              <IconButton
                                size="small"
                                color="error"
                                onClick={() => handleDelete(group)}
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
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
