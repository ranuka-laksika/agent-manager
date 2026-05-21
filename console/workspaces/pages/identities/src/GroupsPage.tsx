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
import { Edit, Folder, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteGroup,
  useListGroups,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import type { ThunderGroup } from "@agent-management-platform/types";

export const GroupsPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);

  const { data, isLoading, error } = useListGroups(
    { orgName: orgId },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteGroup } = useDeleteGroup();
  const { addConfirmation } = useConfirmationDialog();

  const groups = useMemo(() => data?.groups ?? [], [data]);
  const total = data?.total ?? 0;

  const identitiesRoute = (absoluteRouteMap.children.org.children as unknown as {
    identities: { children: { groups: { path: string } } };
  }).identities;

  const createPath = orgId
    ? generatePath(identitiesRoute.children.groups.path + "/create", { orgId })
    : "#";

  const editGroupPath = (groupId: string) =>
    orgId
      ? generatePath(identitiesRoute.children.groups.path + "/:groupId/edit", { orgId, groupId })
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

  if (isLoading) {
    return (
      <PageLayout title="Groups" disableIcon>
        <Box display="flex" justifyContent="center" mt={4}>
          <CircularProgress />
        </Box>
      </PageLayout>
    );
  }

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

      <ListingTable.Container>
        {groups.length === 0 ? (
          <ListingTable.EmptyState
            illustration={<Folder size={64} />}
            title="No groups yet"
            description='Click "Create Group" to add one.'
          />
        ) : (
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Description</ListingTable.Cell>
                <ListingTable.Cell />
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {groups.map((group: ThunderGroup) => (
                <ListingTable.Row key={group.id}>
                  <ListingTable.Cell>{group.name}</ListingTable.Cell>
                  <ListingTable.Cell>{group.description ?? "-"}</ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <Stack direction="row" spacing={0.5} justifyContent="flex-end">
                      <Tooltip title="Edit group">
                        <IconButton size="small" onClick={() => navigate(editGroupPath(group.id))}>
                          <Edit size={16} />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Delete group">
                        <IconButton size="small" onClick={() => handleDelete(group)}>
                          <Trash size={16} />
                        </IconButton>
                      </Tooltip>
                    </Stack>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        )}
        {groups.length > 0 && (
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
